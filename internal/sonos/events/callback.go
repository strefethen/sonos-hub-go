package events

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// CallbackHandler handles UPnP NOTIFY events from Sonos devices.
type CallbackHandler struct {
	manager *Manager
}

// NewCallbackHandler creates a new callback handler.
func NewCallbackHandler(manager *Manager) *CallbackHandler {
	return &CallbackHandler{
		manager: manager,
	}
}

// ServeHTTP handles incoming NOTIFY requests.
func (h *CallbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only accept NOTIFY method
	if r.Method != "NOTIFY" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract headers
	sid := r.Header.Get("SID")
	seq := ParseSEQ(r.Header.Get("SEQ"))
	nt := r.Header.Get("NT")
	nts := r.Header.Get("NTS")

	// Validate headers
	if sid == "" {
		http.Error(w, "Missing SID", http.StatusBadRequest)
		return
	}
	if nt != "upnp:event" {
		http.Error(w, "Invalid NT", http.StatusBadRequest)
		return
	}
	if nts != "upnp:propchange" {
		http.Error(w, "Invalid NTS", http.StatusBadRequest)
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}

	// Extract source IP from request
	sourceIP := extractSourceIP(r)

	// Infer service type from path
	serviceType := InferServiceTypeFromPath(r.URL.Path)

	// Process the event
	if h.manager != nil {
		h.manager.handleNotify(sid, seq, serviceType, sourceIP, body)
	}

	// Always respond 200 OK to acknowledge receipt
	w.WriteHeader(http.StatusOK)
}

// extractSourceIP extracts the source IP from the request.
func extractSourceIP(r *http.Request) string {
	// Try X-Forwarded-For first (if behind proxy)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	// Fall back to RemoteAddr
	addr := r.RemoteAddr
	// RemoteAddr is typically "ip:port", extract just the IP
	if colonIdx := strings.LastIndex(addr, ":"); colonIdx != -1 {
		return addr[:colonIdx]
	}
	return addr
}

// RegisterCallbackRoutes registers the NOTIFY callback routes.
// This should be called during server initialization.
func RegisterCallbackRoutes(mux *http.ServeMux, handler *CallbackHandler) {
	// Register a single callback endpoint
	mux.Handle("/upnp/notify", handler)

	// Also register service-specific paths for clarity
	mux.Handle("/upnp/notify/avtransport", handler)
	mux.Handle("/upnp/notify/renderingcontrol", handler)
	mux.Handle("/upnp/notify/topology", handler)
}

// handleNotify is called by the Manager to process events.
func (m *Manager) handleNotify(sid string, seq int, serviceType ServiceType, sourceIP string, body []byte) {
	m.mu.Lock()
	m.stats.EventsReceived++
	m.mu.Unlock()

	// Find the subscription
	sub := m.findSubscriptionBySID(sid)
	if sub == nil {
		log.Printf("UPNP: Received event for unknown SID: %s", sid)
		return
	}

	// Check sequence number for missed events
	if seq > 0 && seq != sub.SEQ+1 && sub.SEQ > 0 {
		log.Printf("UPNP: Sequence gap detected: expected %d, got %d", sub.SEQ+1, seq)
	}

	// Update sequence number
	m.updateSubscriptionSEQ(sid, seq)

	// Parse the event
	event, err := ParseNotifyBody(body, serviceType)
	if err != nil {
		log.Printf("UPNP: Failed to parse event body: %v", err)
		return
	}

	// Update the state cache
	m.processEvent(event, sourceIP, sub.DeviceUDN)

	m.mu.Lock()
	m.stats.EventsProcessed++
	m.stats.LastEventAt = m.now()
	m.mu.Unlock()
}

// processEvent updates the state cache based on the event.
func (m *Manager) processEvent(event *NotifyEvent, deviceIP string, deviceUDN string) {
	if m.stateCache == nil {
		return
	}

	switch event.ServiceType {
	case ServiceAVTransport:
		avEvent := &AVTransportEvent{
			TransportState:         event.Properties["TransportState"],
			TransportStatus:        event.Properties["TransportStatus"],
			CurrentTrackURI:        event.Properties["CurrentTrackURI"],
			CurrentTrackMetaData:   event.Properties["CurrentTrackMetaData"],
			TrackDuration:          event.Properties["TrackDuration"],
			RelTime:                event.Properties["RelTime"],
			AVTransportURI:         event.Properties["AVTransportURI"],
			AVTransportURIMetaData: event.Properties["AVTransportURIMetaData"],
		}
		m.stateCache.UpdateTransport(deviceIP, avEvent)
		log.Printf("UPNP: AVTransport state updated for %s: %s", deviceIP, avEvent.TransportState)

	case ServiceRenderingControl:
		volume := 0
		muted := false
		if v, ok := event.Properties["Volume"]; ok {
			if vol, err := parseInt(v); err == nil {
				volume = vol
			}
		}
		if v, ok := event.Properties["Mute"]; ok {
			muted = v == "1"
		}
		rcEvent := &RenderingControlEvent{
			Volume: volume,
			Muted:  muted,
		}
		m.stateCache.UpdateVolume(deviceIP, rcEvent)
		log.Printf("UPNP: RenderingControl state updated for %s: vol=%d, mute=%v", deviceIP, volume, muted)

	case ServiceZoneGroupTopology:
		// Invalidate the zone cache when topology changes
		if m.zoneCache != nil {
			m.zoneCache.Invalidate()
			log.Printf("UPNP: Zone topology changed, cache invalidated")
		}
	}

	// Set UDN if we know it
	if deviceUDN != "" {
		m.stateCache.SetUDN(deviceIP, deviceUDN)
	}
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
