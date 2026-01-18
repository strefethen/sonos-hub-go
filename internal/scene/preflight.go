package scene

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/apperrors"
	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

const (
	maxPreflightRetries = 2
	preflightRetryDelay = 500 * time.Millisecond
)

// DetectedIssue represents an issue found during preflight check.
type DetectedIssue struct {
	Type        IssueType
	Details     string
	AutoFixable bool
	RoomName    string
	Diagnostics map[string]any
}

// PreFlightResult contains the result of a preflight check.
type PreFlightResult struct {
	CanProceed   bool
	Issue        *DetectedIssue
	SuggestedFix func() error
}

// PreFlightChecker diagnoses playback issues before scene execution.
type PreFlightChecker struct {
	soapClient *soap.Client
	timeout    time.Duration
	logger     *log.Logger
}

// NewPreFlightChecker creates a new PreFlightChecker.
func NewPreFlightChecker(soapClient *soap.Client, timeout time.Duration, logger *log.Logger) *PreFlightChecker {
	if logger == nil {
		logger = log.Default()
	}
	return &PreFlightChecker{
		soapClient: soapClient,
		timeout:    timeout,
		logger:     logger,
	}
}

// Check performs preflight checks on a device.
func (pf *PreFlightChecker) Check(deviceIP, roomName string, retryCount int) (*PreFlightResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pf.timeout)
	defer cancel()

	// Get transport info - also tests reachability
	transportInfo, err := pf.soapClient.GetTransportInfo(ctx, deviceIP)
	if err != nil {
		// Check for timeout/connection errors
		if isConnectionError(err) {
			return &PreFlightResult{
				CanProceed: false,
				Issue: &DetectedIssue{
					Type:        IssueTypeOffline,
					Details:     fmt.Sprintf("Device at %s is offline or unreachable", deviceIP),
					AutoFixable: false,
					RoomName:    roomName,
					Diagnostics: map[string]any{"error": err.Error()},
				},
			}, nil
		}
		return nil, err
	}

	// Get media info - current URI
	mediaInfo, err := pf.soapClient.GetMediaInfo(ctx, deviceIP)
	if err != nil {
		return nil, err
	}

	// Check 1: TV_MODE - x-sonos-htastream URI
	if strings.Contains(mediaInfo.CurrentURI, "x-sonos-htastream") {
		return &PreFlightResult{
			CanProceed: false,
			Issue: &DetectedIssue{
				Type:        IssueTypeTVMode,
				Details:     fmt.Sprintf("Device %s is in TV mode", roomName),
				AutoFixable: true,
				RoomName:    roomName,
				Diagnostics: map[string]any{
					"currentURI":     mediaInfo.CurrentURI,
					"transportState": transportInfo.CurrentTransportState,
				},
			},
			SuggestedFix: pf.createTVModeFix(deviceIP),
		}, nil
	}

	// Check 2: NOT_COORDINATOR - x-rincon: URI means device is grouped with another
	rinconPattern := regexp.MustCompile(`^x-rincon:(RINCON_[A-F0-9]+)`)
	if rinconPattern.MatchString(mediaInfo.CurrentURI) {
		matches := rinconPattern.FindStringSubmatch(mediaInfo.CurrentURI)
		coordinatorUUID := ""
		if len(matches) > 1 {
			coordinatorUUID = matches[1]
		}
		return &PreFlightResult{
			CanProceed: false,
			Issue: &DetectedIssue{
				Type:        IssueTypeNotCoordinator,
				Details:     fmt.Sprintf("Device %s is grouped with another speaker", roomName),
				AutoFixable: true,
				RoomName:    roomName,
				Diagnostics: map[string]any{
					"currentURI":      mediaInfo.CurrentURI,
					"coordinatorUUID": coordinatorUUID,
				},
			},
			SuggestedFix: pf.createUngroupFix(deviceIP),
		}, nil
	}

	// Check 3: TRANSITIONING - retry with backoff
	if transportInfo.CurrentTransportState == "TRANSITIONING" {
		if retryCount < maxPreflightRetries {
			pf.logger.Printf("Device %s is transitioning, retrying in %v (attempt %d/%d)",
				roomName, preflightRetryDelay, retryCount+1, maxPreflightRetries)
			time.Sleep(preflightRetryDelay)
			return pf.Check(deviceIP, roomName, retryCount+1)
		}
		return &PreFlightResult{
			CanProceed: false,
			Issue: &DetectedIssue{
				Type:        IssueTypeTransitioning,
				Details:     fmt.Sprintf("Device %s is transitioning and did not stabilize", roomName),
				AutoFixable: false,
				RoomName:    roomName,
				Diagnostics: map[string]any{
					"transportState": transportInfo.CurrentTransportState,
					"retryCount":     retryCount,
				},
			},
		}, nil
	}

	// All checks passed
	return &PreFlightResult{
		CanProceed: true,
	}, nil
}

// AttemptAutoFix tries to fix a detected issue.
func (pf *PreFlightChecker) AttemptAutoFix(result *PreFlightResult) bool {
	if result == nil || result.Issue == nil || !result.Issue.AutoFixable || result.SuggestedFix == nil {
		return false
	}

	pf.logger.Printf("Attempting auto-fix for %s issue on %s",
		result.Issue.Type, result.Issue.RoomName)

	if err := result.SuggestedFix(); err != nil {
		pf.logger.Printf("Auto-fix failed: %v", err)
		return false
	}

	pf.logger.Printf("Auto-fix succeeded for %s", result.Issue.RoomName)
	return true
}

// CreateError creates an AppError from a detected issue.
func (pf *PreFlightChecker) CreateError(issue *DetectedIssue) *apperrors.AppError {
	var message string
	var suggestion string
	var code apperrors.ErrorCode

	switch issue.Type {
	case IssueTypeTVMode:
		code = apperrors.ErrorCodeSonosRejected
		message = fmt.Sprintf("Speaker '%s' is in TV mode", issue.RoomName)
		suggestion = "Please use the Sonos app to switch from TV to music"

	case IssueTypeNotCoordinator:
		code = apperrors.ErrorCodeSonosRejected
		message = fmt.Sprintf("Speaker '%s' is grouped with another speaker", issue.RoomName)
		suggestion = "Please ungroup the speaker in the Sonos app"

	case IssueTypeTransitioning:
		code = apperrors.ErrorCodeSonosRejected
		message = fmt.Sprintf("Speaker '%s' is transitioning", issue.RoomName)
		suggestion = "Wait a moment and try again"

	case IssueTypeOffline:
		code = apperrors.ErrorCodeDeviceOffline
		message = fmt.Sprintf("Speaker '%s' is offline or unreachable", issue.RoomName)
		suggestion = "Check that the speaker is powered on and connected to the network"

	case IssueTypeNoPlayAction:
		code = apperrors.ErrorCodeSonosRejected
		message = fmt.Sprintf("Speaker '%s' cannot play right now", issue.RoomName)
		suggestion = "Try selecting music in the Sonos app first"

	default:
		code = apperrors.ErrorCodeSonosRejected
		message = fmt.Sprintf("Preflight check failed for speaker '%s'", issue.RoomName)
		suggestion = "Check the speaker status in the Sonos app"
	}

	return apperrors.NewAppError(
		code,
		message,
		502,
		map[string]any{
			"issue_type":  string(issue.Type),
			"room_name":   issue.RoomName,
			"diagnostics": issue.Diagnostics,
		},
		&apperrors.Remediation{
			UserAction: suggestion,
		},
	)
}

// createTVModeFix creates a function to fix TV mode by switching to queue.
func (pf *PreFlightChecker) createTVModeFix(deviceIP string) func() error {
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), pf.timeout)
		defer cancel()

		// Get device UUID for queue URI
		zoneAttrs, err := pf.soapClient.GetZoneAttributes(ctx, deviceIP)
		if err != nil {
			// Fall back to using a generic queue switch
			// This uses an empty URI to exit TV mode
			if err := pf.soapClient.SetAVTransportURI(ctx, deviceIP, "", ""); err != nil {
				return fmt.Errorf("failed to exit TV mode: %w", err)
			}
			time.Sleep(200 * time.Millisecond)
			return nil
		}

		// Build queue URI using zone name (Sonos typically uses RINCON format)
		// For now, just set an empty URI to exit TV mode
		_ = zoneAttrs
		if err := pf.soapClient.SetAVTransportURI(ctx, deviceIP, "", ""); err != nil {
			return fmt.Errorf("failed to switch to queue: %w", err)
		}

		time.Sleep(200 * time.Millisecond)
		return nil
	}
}

// createUngroupFix creates a function to make device a standalone coordinator.
func (pf *PreFlightChecker) createUngroupFix(deviceIP string) func() error {
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), pf.timeout)
		defer cancel()

		if err := pf.soapClient.BecomeCoordinatorOfStandaloneGroup(ctx, deviceIP); err != nil {
			return fmt.Errorf("failed to ungroup: %w", err)
		}

		time.Sleep(300 * time.Millisecond)
		return nil
	}
}

// isConnectionError checks if an error indicates a connection problem.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	// Check for Sonos-specific error types
	var timeoutErr *soap.SonosTimeoutError
	if errors.As(err, &timeoutErr) {
		return true
	}

	var unreachableErr *soap.SonosUnreachableError
	if errors.As(err, &unreachableErr) {
		return true
	}

	// Check error message for common connection errors
	errStr := err.Error()
	connectionPatterns := []string{
		"timeout",
		"connection refused",
		"no route to host",
		"network is unreachable",
		"i/o timeout",
		"dial tcp",
		"ECONNREFUSED",
		"ETIMEDOUT",
		"EHOSTUNREACH",
	}

	for _, pattern := range connectionPatterns {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}
