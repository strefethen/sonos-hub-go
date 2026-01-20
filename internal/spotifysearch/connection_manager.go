package spotifysearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var (
	// ErrExtensionNotConnected is returned when no extension is connected
	ErrExtensionNotConnected = errors.New("Spotify search extension not connected")
	// ErrSearchTimeout is returned when a search times out
	ErrSearchTimeout = errors.New("Search timed out")
	// ErrExtensionDisconnected is returned when the extension disconnects during a search
	ErrExtensionDisconnected = errors.New("Extension disconnected")
)

type pendingSearch struct {
	requestID    string
	query        string
	contentTypes []SpotifyContentType
	resultCh     chan searchResponse
	createdAt    time.Time
}

type searchResponse struct {
	results *GroupedSearchResults
	err     error
}

// ConnectionManager manages the WebSocket connection to the Spotify search extension
type ConnectionManager struct {
	mu              sync.RWMutex
	conn            *websocket.Conn
	pendingSearches map[string]*pendingSearch
	requestCounter  uint64
	searchTimeout   time.Duration
	pingInterval    time.Duration

	// For cleanup
	stopPing chan struct{}
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		pendingSearches: make(map[string]*pendingSearch),
		searchTimeout:   15 * time.Second,
		pingInterval:    30 * time.Second,
	}
}

// SetConnection registers a new WebSocket connection from the extension
func (m *ConnectionManager) SetConnection(conn *websocket.Conn) {
	m.mu.Lock()

	// Close existing connection if any
	if m.conn != nil {
		m.conn.Close()
	}
	if m.stopPing != nil {
		close(m.stopPing)
	}

	m.conn = conn
	m.stopPing = make(chan struct{})
	m.mu.Unlock()

	// Start ping interval
	go m.startPingLoop()

	// Start message reader
	go m.readMessages()

	log.Printf("Spotify search extension connected")
}

func (m *ConnectionManager) startPingLoop() {
	ticker := time.NewTicker(m.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.mu.RLock()
			conn := m.conn
			m.mu.RUnlock()

			if conn != nil {
				msg := PingMessage{Type: "ping"}
				m.mu.Lock()
				err := m.conn.WriteJSON(msg)
				m.mu.Unlock()
				if err != nil {
					log.Printf("Failed to send ping: %v", err)
				}
			}
		case <-m.stopPing:
			return
		}
	}
}

func (m *ConnectionManager) readMessages() {
	m.mu.RLock()
	conn := m.conn
	m.mu.RUnlock()

	if conn == nil {
		return
	}

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			m.handleDisconnect()
			return
		}

		m.handleMessage(message)
	}
}

func (m *ConnectionManager) handleMessage(message []byte) {
	// First parse just the type
	var incoming IncomingMessage
	if err := json.Unmarshal(message, &incoming); err != nil {
		log.Printf("Failed to parse WebSocket message: %v", err)
		return
	}

	switch incoming.Type {
	case "pong":
		// Keepalive response, nothing to do
		return
	case "searchResult":
		var result SearchResultMessage
		if err := json.Unmarshal(message, &result); err != nil {
			log.Printf("Failed to parse search result: %v", err)
			return
		}
		m.handleSearchResult(&result)
	default:
		log.Printf("Unknown message type: %s", incoming.Type)
	}
}

func (m *ConnectionManager) handleSearchResult(result *SearchResultMessage) {
	m.mu.Lock()
	pending, exists := m.pendingSearches[result.RequestID]
	if exists {
		delete(m.pendingSearches, result.RequestID)
	}
	m.mu.Unlock()

	if !exists {
		log.Printf("Received result for unknown request: %s", result.RequestID)
		return
	}

	duration := time.Since(pending.createdAt)

	if result.Error != "" {
		log.Printf("Search failed for query '%s': %s (took %v)", pending.query, result.Error, duration)
		pending.resultCh <- searchResponse{err: errors.New(result.Error)}
	} else {
		log.Printf("Search completed for query '%s' (took %v)", pending.query, duration)
		pending.resultCh <- searchResponse{results: &result.Results}
	}
}

func (m *ConnectionManager) handleDisconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Printf("Spotify search extension disconnected")

	m.conn = nil
	if m.stopPing != nil {
		// Use a select to avoid panic if channel is already closed
		select {
		case <-m.stopPing:
			// Already closed
		default:
			close(m.stopPing)
		}
		m.stopPing = nil
	}

	// Reject all pending searches
	for _, pending := range m.pendingSearches {
		pending.resultCh <- searchResponse{err: ErrExtensionDisconnected}
	}
	m.pendingSearches = make(map[string]*pendingSearch)
}

// IsConnected returns whether the extension is connected
func (m *ConnectionManager) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.conn != nil
}

// GetStatus returns the current connection status
func (m *ConnectionManager) GetStatus() ConnectionStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := "disconnected"
	if m.conn != nil {
		status = "connected"
	}

	return ConnectionStatus{
		Extension:       status,
		PendingSearches: len(m.pendingSearches),
	}
}

// Search performs a search via the extension
func (m *ConnectionManager) Search(ctx context.Context, query string, contentTypes []SpotifyContentType) (*GroupedSearchResults, error) {
	m.mu.RLock()
	conn := m.conn
	m.mu.RUnlock()

	if conn == nil {
		return nil, ErrExtensionNotConnected
	}

	// Generate request ID
	requestID := fmt.Sprintf("search_%d", atomic.AddUint64(&m.requestCounter, 1))

	// Create pending search
	pending := &pendingSearch{
		requestID:    requestID,
		query:        query,
		contentTypes: contentTypes,
		resultCh:     make(chan searchResponse, 1),
		createdAt:    time.Now(),
	}

	m.mu.Lock()
	m.pendingSearches[requestID] = pending
	m.mu.Unlock()

	// Send search request
	request := SearchRequest{
		Type:         "search",
		RequestID:    requestID,
		Query:        query,
		ContentTypes: contentTypes,
	}

	m.mu.Lock()
	err := m.conn.WriteJSON(request)
	m.mu.Unlock()

	if err != nil {
		m.mu.Lock()
		delete(m.pendingSearches, requestID)
		m.mu.Unlock()
		return nil, fmt.Errorf("failed to send search request: %w", err)
	}

	log.Printf("Sent search request for query '%s' (requestId: %s)", query, requestID)

	// Wait for result with timeout
	select {
	case <-ctx.Done():
		m.mu.Lock()
		delete(m.pendingSearches, requestID)
		m.mu.Unlock()
		return nil, ctx.Err()
	case <-time.After(m.searchTimeout):
		m.mu.Lock()
		delete(m.pendingSearches, requestID)
		m.mu.Unlock()
		return nil, ErrSearchTimeout
	case response := <-pending.resultCh:
		return response.results, response.err
	}
}

// Close closes the connection manager
func (m *ConnectionManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn != nil {
		m.conn.Close()
		m.conn = nil
	}
	if m.stopPing != nil {
		// Use a select to avoid panic if channel is already closed
		select {
		case <-m.stopPing:
			// Already closed
		default:
			close(m.stopPing)
		}
		m.stopPing = nil
	}
}
