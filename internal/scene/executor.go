package scene

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/devices"
	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

// Executor orchestrates scene execution.
type Executor struct {
	logger         *log.Logger
	execRepo       *ExecutionsRepository
	lock           *CoordinatorLock
	preflight      *PreFlightChecker
	deviceService  *devices.Service
	soapClient     *soap.Client
	timeout        time.Duration
	commandTimeout time.Duration         // Short timeout for commands (3s)
	monitorConfig  PlaybackMonitorConfig // Monitoring configuration
}

// NewExecutor creates a new Executor.
func NewExecutor(
	logger *log.Logger,
	execRepo *ExecutionsRepository,
	lock *CoordinatorLock,
	preflight *PreFlightChecker,
	deviceService *devices.Service,
	soapClient *soap.Client,
	timeout time.Duration,
) *Executor {
	if logger == nil {
		logger = log.Default()
	}
	return &Executor{
		logger:         logger,
		execRepo:       execRepo,
		lock:           lock,
		preflight:      preflight,
		deviceService:  deviceService,
		soapClient:     soapClient,
		timeout:        timeout,
		commandTimeout: 3 * time.Second,
		monitorConfig: PlaybackMonitorConfig{
			MaxWaitTime:       30 * time.Second,
			InitialPollDelay:  500 * time.Millisecond,
			MaxPollDelay:      3 * time.Second,
			BackoffMultiplier: 1.5,
			TransitionTimeout: 15 * time.Second,
		},
	}
}

// Execute runs a scene execution through all steps.
func (e *Executor) Execute(scene *Scene, execution *SceneExecution, options ExecuteOptions) (*SceneExecution, error) {
	var coordinatorIP string
	var coordinatorUDN string
	var lockAcquired bool

	// Ensure lock is released on exit
	defer func() {
		if lockAcquired && coordinatorUDN != "" {
			e.lock.Unlock(coordinatorUDN)
			e.updateStep(execution.SceneExecutionID, "release_lock", StepStatusCompleted, nil, nil)
		}
	}()

	// Step 1: Determine coordinator
	e.updateStep(execution.SceneExecutionID, "determine_coordinator", StepStatusRunning, nil, nil)
	coordinator, err := e.determineCoordinator(scene, options)
	if err != nil {
		e.updateStep(execution.SceneExecutionID, "determine_coordinator", StepStatusFailed, &err, nil)
		return e.failExecution(execution, err)
	}
	coordinatorIP = coordinator.IP
	coordinatorUDN = coordinator.UDN
	if err := e.execRepo.SetCoordinator(execution.SceneExecutionID, coordinatorUDN); err != nil {
		e.logger.Printf("Failed to set coordinator: %v", err)
	}
	e.updateStep(execution.SceneExecutionID, "determine_coordinator", StepStatusCompleted, nil, map[string]any{
		"coordinator_udn": coordinatorUDN,
		"coordinator_ip":  coordinatorIP,
	})

	// Step 2: Acquire lock
	e.updateStep(execution.SceneExecutionID, "acquire_lock", StepStatusRunning, nil, nil)
	if !e.lock.TryLock(coordinatorUDN) {
		err := fmt.Errorf("coordinator %s is locked by another execution", coordinatorUDN)
		e.updateStep(execution.SceneExecutionID, "acquire_lock", StepStatusFailed, &err, nil)
		return e.failExecution(execution, err)
	}
	lockAcquired = true
	e.updateStep(execution.SceneExecutionID, "acquire_lock", StepStatusCompleted, nil, nil)

	// Step 3: Ensure group
	e.updateStep(execution.SceneExecutionID, "ensure_group", StepStatusRunning, nil, nil)
	groupResults := e.ensureGroup(scene, coordinatorIP, coordinatorUDN)
	e.updateStep(execution.SceneExecutionID, "ensure_group", StepStatusCompleted, nil, map[string]any{
		"results": groupResults,
	})

	// Step 4: Apply volume
	e.updateStep(execution.SceneExecutionID, "apply_volume", StepStatusRunning, nil, nil)
	volumeResults := e.applyVolume(scene)
	e.updateStep(execution.SceneExecutionID, "apply_volume", StepStatusCompleted, nil, map[string]any{
		"results": volumeResults,
	})

	// Step 5: Pre-flight check
	e.updateStep(execution.SceneExecutionID, "pre_flight_check", StepStatusRunning, nil, nil)
	if err := e.runPreFlightWithRecovery(coordinatorIP, coordinator.RoomName, options.TVPolicy); err != nil {
		e.updateStep(execution.SceneExecutionID, "pre_flight_check", StepStatusFailed, &err, nil)
		return e.failExecution(execution, err)
	}
	e.updateStep(execution.SceneExecutionID, "pre_flight_check", StepStatusCompleted, nil, nil)

	// Step 6: Start playback (fire-and-forget with short timeout)
	e.updateStep(execution.SceneExecutionID, "start_playback", StepStatusRunning, nil, nil)
	expectedContent, err := e.startPlayback(coordinatorIP, coordinatorUDN, options)
	if err != nil {
		e.updateStep(execution.SceneExecutionID, "start_playback", StepStatusFailed, &err, nil)
		return e.failExecution(execution, err)
	}
	startPlaybackDetails := map[string]any{}
	if expectedContent != nil {
		startPlaybackDetails["uses_queue"] = expectedContent.UsesQueue
		if expectedContent.URI != "" {
			startPlaybackDetails["uri"] = expectedContent.URI
		}
		if expectedContent.QueueURI != "" {
			startPlaybackDetails["queue_uri"] = expectedContent.QueueURI
		}
	}
	e.updateStep(execution.SceneExecutionID, "start_playback", StepStatusCompleted, nil, startPlaybackDetails)

	// Step 7: Verify playback (with monitoring/polling)
	e.updateStep(execution.SceneExecutionID, "verify_playback", StepStatusRunning, nil, nil)
	// Create a context with the overall monitoring timeout
	monitorCtx, monitorCancel := context.WithTimeout(context.Background(), e.monitorConfig.MaxWaitTime+5*time.Second)
	defer monitorCancel()
	verification := e.verifyPlayback(monitorCtx, coordinatorIP, expectedContent)

	verifyDetails := map[string]any{
		"playback_confirmed": verification.PlaybackConfirmed,
		"transport_state":    verification.TransportState,
		"attempts":           verification.Attempts,
		"duration_ms":        verification.DurationMs,
	}
	if verification.FailureReason != "" {
		verifyDetails["failure_reason"] = string(verification.FailureReason)
		verifyDetails["failure_message"] = verification.FailureMessage
	}
	if verification.TrackURI != "" {
		verifyDetails["track_uri"] = verification.TrackURI
	}
	if verification.DataSource != "" {
		verifyDetails["data_source"] = verification.DataSource
	}
	e.updateStep(execution.SceneExecutionID, "verify_playback", StepStatusCompleted, nil, verifyDetails)

	// Complete execution
	status := ExecutionStatusPlayingConfirmed
	if !verification.PlaybackConfirmed && !verification.VerificationUnavailable {
		status = ExecutionStatusFailed
	}
	if err := e.execRepo.Complete(execution.SceneExecutionID, status, &verification, nil); err != nil {
		e.logger.Printf("Failed to complete execution: %v", err)
	}

	// Release lock (will be done in defer, but mark step complete)
	e.updateStep(execution.SceneExecutionID, "release_lock", StepStatusRunning, nil, nil)
	// Lock release happens in defer

	return e.execRepo.GetByID(execution.SceneExecutionID)
}

// coordinatorInfo holds resolved coordinator information.
type coordinatorInfo struct {
	UDN      string
	IP       string
	RoomName string
}

// determineCoordinator finds the best coordinator for a scene.
func (e *Executor) determineCoordinator(scene *Scene, options ExecuteOptions) (*coordinatorInfo, error) {
	if len(scene.Members) == 0 {
		return nil, fmt.Errorf("scene has no members")
	}

	// For ARC_FIRST preference, look for Arc/Beam/Ray in topology
	if scene.CoordinatorPreference == string(CoordinatorPreferenceArcFirst) {
		topology, err := e.deviceService.GetTopology()
		if err == nil {
			for _, device := range topology.Devices {
				// Check if device is an Arc/Beam/Ray
				if isArcBeamRay(device.Model) {
					// Check if it's a scene member
					for _, member := range scene.Members {
						if member.UDN == device.UDN || member.RoomName == device.RoomName {
							// Check if it's in TV mode and policy says skip
							if options.TVPolicy == TVPolicySkip {
								// Check media info for TV mode
								ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
								mediaInfo, err := e.soapClient.GetMediaInfo(ctx, device.IP)
								cancel()
								if err == nil && strings.Contains(mediaInfo.CurrentURI, "x-sonos-htastream") {
									e.logger.Printf("Skipping Arc %s due to TV mode and SKIP policy", device.RoomName)
									continue
								}
							}
							return &coordinatorInfo{
								UDN:      device.UDN,
								IP:       device.IP,
								RoomName: device.RoomName,
							}, nil
						}
					}
				}
			}
		}
	}

	// Fallback: use first member
	firstMember := scene.Members[0]
	ip, err := e.resolveMemberIP(firstMember)
	if err != nil {
		return nil, err
	}

	roomName := firstMember.RoomName
	if roomName == "" {
		roomName = firstMember.UDN
	}

	return &coordinatorInfo{
		UDN:      firstMember.UDN,
		IP:       ip,
		RoomName: roomName,
	}, nil
}

// ensureGroup joins all members to the coordinator.
func (e *Executor) ensureGroup(scene *Scene, coordinatorIP, coordinatorUDN string) []map[string]any {
	var results []map[string]any

	// coordinatorUDN is already a RINCON_ format UDN
	coordinatorUUID := coordinatorUDN

	for _, member := range scene.Members {
		if member.UDN == coordinatorUDN {
			results = append(results, map[string]any{
				"udn":     member.UDN,
				"skipped": true,
				"reason":  "is_coordinator",
			})
			continue
		}

		memberIP, err := e.resolveMemberIP(member)
		if err != nil {
			results = append(results, map[string]any{
				"udn":     member.UDN,
				"success": false,
				"error":   err.Error(),
			})
			continue
		}

		// Join to coordinator group
		groupURI := fmt.Sprintf("x-rincon:%s", coordinatorUUID)
		ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
		err = e.soapClient.SetAVTransportURI(ctx, memberIP, groupURI, "")
		cancel()

		if err != nil {
			results = append(results, map[string]any{
				"udn":     member.UDN,
				"success": false,
				"error":   err.Error(),
			})
		} else {
			results = append(results, map[string]any{
				"udn":     member.UDN,
				"success": true,
			})
		}
	}

	return results
}

// applyVolume sets target volumes on members.
func (e *Executor) applyVolume(scene *Scene) []map[string]any {
	var results []map[string]any

	for _, member := range scene.Members {
		if member.TargetVolume == nil {
			continue
		}

		memberIP, err := e.resolveMemberIP(member)
		if err != nil {
			results = append(results, map[string]any{
				"udn":     member.UDN,
				"success": false,
				"error":   err.Error(),
			})
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
		err = e.soapClient.SetVolume(ctx, memberIP, *member.TargetVolume)
		cancel()

		if err != nil {
			results = append(results, map[string]any{
				"udn":     member.UDN,
				"success": false,
				"error":   err.Error(),
			})
		} else {
			results = append(results, map[string]any{
				"udn":     member.UDN,
				"success": true,
				"volume":  *member.TargetVolume,
			})
		}
	}

	return results
}

// runPreFlightWithRecovery runs preflight check with auto-fix attempts.
func (e *Executor) runPreFlightWithRecovery(coordinatorIP, roomName string, tvPolicy TVPolicy) error {
	result, err := e.preflight.Check(coordinatorIP, roomName, 0)
	if err != nil {
		return err
	}

	if result.CanProceed {
		return nil
	}

	// Handle TV mode based on policy
	if result.Issue != nil && result.Issue.Type == IssueTypeTVMode {
		switch tvPolicy {
		case TVPolicySkip:
			return fmt.Errorf("routine skipped: TV mode active on %s", roomName)
		case TVPolicyUseFallback:
			e.logger.Printf("TV mode on %s, fallback not yet implemented", roomName)
			// For now, proceed with auto-fix attempt
		case TVPolicyAlwaysPlay:
			// Proceed with auto-fix
		}
	}

	// Attempt auto-fix if possible
	if result.Issue != nil && result.Issue.AutoFixable {
		if e.preflight.AttemptAutoFix(result) {
			// Re-check after fix
			result, err = e.preflight.Check(coordinatorIP, roomName, 0)
			if err != nil {
				return err
			}
			if result.CanProceed {
				return nil
			}
		}
	}

	// Create error from issue
	if result.Issue != nil {
		return e.preflight.CreateError(result.Issue)
	}

	return fmt.Errorf("preflight check failed for %s", roomName)
}

// startPlayback starts playback on the coordinator using fire-and-forget pattern.
// Returns ExpectedContent for verification, only errors on device unreachable (hard failures).
// Timeout errors are acceptable - we verify via polling afterward.
//
// IMPORTANT: Each operation gets its own timeout context to prevent slow operations
// (like AddURIToQueue for podcasts which can take 2-3s) from consuming the timeout
// for subsequent operations like Play.
func (e *Executor) startPlayback(coordinatorIP, coordinatorUDN string, options ExecuteOptions) (*ExpectedContent, error) {
	expected := &ExpectedContent{}

	// If music content is provided, set it up first
	if options.MusicContent != nil && options.MusicContent.URI != "" {
		expected.URI = options.MusicContent.URI
		expected.UsesQueue = options.MusicContent.UsesQueue

		if options.MusicContent.UsesQueue {
			// Queue-based playback for containers (playlists, albums, podcasts)
			e.logger.Printf("Using queue-based playback for container content")

			// Clear queue first (best effort - log but continue) - own timeout
			clearCtx, clearCancel := context.WithTimeout(context.Background(), e.commandTimeout)
			if err := e.clearQueueWithRetry(clearCtx, coordinatorIP); err != nil {
				e.logger.Printf("Warning: failed to clear queue: %v", err)
			}
			clearCancel()

			// Add content to queue - own timeout (this is the slow one for podcasts)
			// timeout OK, device unreachable is fatal
			addCtx, addCancel := context.WithTimeout(context.Background(), e.commandTimeout)
			_, err := e.soapClient.AddURIToQueue(addCtx, coordinatorIP,
				options.MusicContent.URI, options.MusicContent.Metadata, 0, false)
			addCancel()
			if err != nil {
				if isDeviceUnreachableError(err) {
					return nil, fmt.Errorf("device unreachable: %w", err)
				}
				if !isTimeoutError(err) {
					// Log non-timeout errors but continue - will verify via polling
					e.logger.Printf("Warning: AddURIToQueue error (will verify): %v", err)
				}
			}

			// Set transport to the queue - own timeout
			expected.QueueURI = fmt.Sprintf("x-rincon-queue:%s#0", coordinatorUDN)
			setCtx, setCancel := context.WithTimeout(context.Background(), e.commandTimeout)
			err = e.soapClient.SetAVTransportURI(setCtx, coordinatorIP, expected.QueueURI, "")
			setCancel()
			if err != nil && isDeviceUnreachableError(err) {
				return nil, fmt.Errorf("device unreachable: %w", err)
			}
		} else {
			// Direct playback for tracks and stations
			e.logger.Printf("Using direct playback for track/station content")

			// Clear queue first if replacing - own timeout
			if options.QueueMode == QueueModeReplaceAndPlay {
				clearCtx, clearCancel := context.WithTimeout(context.Background(), e.commandTimeout)
				if err := e.clearQueueWithRetry(clearCtx, coordinatorIP); err != nil {
					e.logger.Printf("Warning: failed to clear queue: %v", err)
				}
				clearCancel()
			}

			// Set the URI directly - own timeout
			setCtx, setCancel := context.WithTimeout(context.Background(), e.commandTimeout)
			err := e.soapClient.SetAVTransportURI(setCtx, coordinatorIP,
				options.MusicContent.URI, options.MusicContent.Metadata)
			setCancel()
			if err != nil && isDeviceUnreachableError(err) {
				return nil, fmt.Errorf("device unreachable: %w", err)
			}
		}
	} else if options.FavoriteID != "" {
		// Legacy: play favorite by ID (would need to resolve favorite URI)
		e.logger.Printf("Playing favorite %s (legacy mode)", options.FavoriteID)
	}

	// Send play command - ALWAYS gets fresh timeout regardless of prior operations
	playCtx, playCancel := context.WithTimeout(context.Background(), e.commandTimeout)
	defer playCancel()
	if err := e.soapClient.Play(playCtx, coordinatorIP); err != nil {
		if isDeviceUnreachableError(err) {
			return nil, fmt.Errorf("device unreachable: %w", err)
		}
		e.logger.Printf("Play command error (will verify): %v", err)
	}

	return expected, nil
}

// clearQueueWithRetry clears the queue, retrying on error 800.
func (e *Executor) clearQueueWithRetry(ctx context.Context, ip string) error {
	err := e.soapClient.RemoveAllTracksFromQueue(ctx, ip)
	if err != nil {
		// Check for error 800 (invalid state)
		if strings.Contains(err.Error(), "800") {
			e.logger.Printf("Got error 800, stopping first then retrying clear")
			_ = e.soapClient.Stop(ctx, ip)
			return e.soapClient.RemoveAllTracksFromQueue(ctx, ip)
		}
		return err
	}
	return nil
}

// isTimeoutError checks if an error is a context timeout or deadline exceeded.
// Timeouts are acceptable in fire-and-forget mode - we'll verify via polling.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) ||
		strings.Contains(err.Error(), "timeout")
}

// isDeviceUnreachableError checks for hard failures where the device can't be reached.
func isDeviceUnreachableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no route to host") ||
		strings.Contains(errStr, "network is unreachable")
}

// fetchPlaybackState gets current device state via SOAP for fresh data during monitoring.
// We don't use cache here because monitoring needs real-time state, not 30-second-old data.
func (e *Executor) fetchPlaybackState(ctx context.Context, ip string) (*PlaybackState, error) {
	transportInfo, err := e.soapClient.GetTransportInfo(ctx, ip)
	if err != nil {
		return nil, fmt.Errorf("GetTransportInfo: %w", err)
	}

	positionInfo, _ := e.soapClient.GetPositionInfo(ctx, ip)
	mediaInfo, _ := e.soapClient.GetMediaInfo(ctx, ip)

	return &PlaybackState{
		TransportState: transportInfo.CurrentTransportState,
		TrackURI:       positionInfo.TrackURI,
		AVTransportURI: mediaInfo.CurrentURI,
		ObservedAt:     time.Now(),
		Source:         "soap",
	}, nil
}

// isExpectedContentPlaying checks if the observed state matches expected content.
func (e *Executor) isExpectedContentPlaying(state *PlaybackState, expected *ExpectedContent) bool {
	// Must be in PLAYING state
	if state.TransportState != "PLAYING" {
		return false
	}

	// TV mode check - htastream means TV audio, not our content
	if strings.Contains(state.AVTransportURI, "x-sonos-htastream") ||
		strings.Contains(state.TrackURI, "x-sonos-htastream") {
		return false
	}

	if expected == nil {
		// No expected content specified - any playback counts as success
		return true
	}

	if expected.UsesQueue {
		// For queue-based playback, verify transport points to OUR queue
		// QueueURI format: x-rincon-queue:RINCON_xxx#0
		if expected.QueueURI != "" {
			return state.AVTransportURI == expected.QueueURI
		}
		// Fallback: any queue is acceptable if we didn't capture the expected queue URI
		return strings.HasPrefix(state.AVTransportURI, "x-rincon-queue:")
	}

	// For direct playback, match URI (with flexibility for encoding differences)
	if expected.URI != "" {
		expectedBase := strings.Split(expected.URI, "?")[0]
		trackBase := strings.Split(state.TrackURI, "?")[0]
		transportBase := strings.Split(state.AVTransportURI, "?")[0]
		return trackBase == expectedBase || transportBase == expectedBase
	}

	// No expected URI - any non-TV playback counts
	return state.TrackURI != ""
}

// monitorPlayback polls device state until playback is confirmed or failure detected.
// Accepts a parent context for cancellation (e.g., if HTTP request is cancelled).
func (e *Executor) monitorPlayback(parentCtx context.Context, coordinatorIP string, expected *ExpectedContent) *PlaybackResult {
	result := &PlaybackResult{}
	startTime := time.Now()
	pollDelay := e.monitorConfig.InitialPollDelay // 500ms
	transitionStart := time.Time{}
	stoppedCount := 0 // Track consecutive STOPPED states

	// Helper to set duration on all exit paths
	setDuration := func() {
		result.Duration = time.Since(startTime)
	}
	defer setDuration()

	for {
		// Check for context cancellation
		select {
		case <-parentCtx.Done():
			result.FailureReason = FailureReasonTimeout
			result.FailureMessage = "monitoring cancelled"
			return result
		default:
		}

		elapsed := time.Since(startTime)
		if elapsed > e.monitorConfig.MaxWaitTime { // 30s
			result.FailureReason = FailureReasonTimeout
			result.FailureMessage = fmt.Sprintf("playback not confirmed after %v", elapsed)
			return result
		}

		if result.Attempts > 0 {
			time.Sleep(pollDelay)
			pollDelay = time.Duration(float64(pollDelay) * e.monitorConfig.BackoffMultiplier)
			if pollDelay > e.monitorConfig.MaxPollDelay {
				pollDelay = e.monitorConfig.MaxPollDelay // 3s cap
			}
		}
		result.Attempts++

		ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
		state, err := e.fetchPlaybackState(ctx, coordinatorIP)
		cancel()

		if err != nil {
			e.logger.Printf("Poll %d: error fetching state: %v", result.Attempts, err)
			if result.Attempts >= 5 {
				result.FailureReason = FailureReasonDeviceOffline
				result.FailureMessage = err.Error()
				return result
			}
			continue
		}

		result.FinalState = state
		e.logger.Printf("Poll %d: state=%s avTransport=%s track=%s",
			result.Attempts, state.TransportState, state.AVTransportURI, state.TrackURI)

		// TV mode check - early exit (isExpectedContentPlaying also checks, but explicit is clearer)
		if strings.Contains(state.AVTransportURI, "x-sonos-htastream") {
			result.FailureReason = FailureReasonTVModeActive
			result.FailureMessage = "device is in TV mode"
			return result
		}

		// Success check
		if e.isExpectedContentPlaying(state, expected) {
			result.Success = true
			e.logger.Printf("Playback confirmed after %d polls", result.Attempts)
			return result
		}

		// Handle TRANSITIONING state
		if state.TransportState == "TRANSITIONING" {
			stoppedCount = 0 // Reset stopped counter
			if transitionStart.IsZero() {
				transitionStart = time.Now()
				e.logger.Printf("Device transitioning, waiting...")
			} else if time.Since(transitionStart) > e.monitorConfig.TransitionTimeout {
				result.FailureReason = FailureReasonStuckTransitioning
				result.FailureMessage = fmt.Sprintf("stuck transitioning for %v", time.Since(transitionStart))
				return result
			}
			continue
		}
		transitionStart = time.Time{} // Reset transition timer

		// Handle STOPPED state - wait for multiple consecutive before failing
		// Device may briefly stop between loading content
		if state.TransportState == "STOPPED" {
			stoppedCount++
			if stoppedCount >= 4 && elapsed > 5*time.Second {
				// Only fail after multiple STOPPED states AND some time has passed
				result.FailureReason = FailureReasonPlaybackStopped
				result.FailureMessage = "device stopped and did not resume"
				return result
			}
			continue
		}
		stoppedCount = 0 // Reset if not stopped

		// PAUSED_PLAYBACK - unusual but possible, keep waiting
		if state.TransportState == "PAUSED_PLAYBACK" {
			e.logger.Printf("Device paused, waiting for playback...")
		}
	}
}

// verifyPlayback monitors and verifies that playback started correctly.
// The context allows cancellation if the caller needs to abort early.
func (e *Executor) verifyPlayback(ctx context.Context, coordinatorIP string, expected *ExpectedContent) Verification {
	result := e.monitorPlayback(ctx, coordinatorIP, expected)

	v := Verification{
		CheckedAt:         time.Now().UTC(),
		PlaybackConfirmed: result.Success,
		Attempts:          result.Attempts,
		DurationMs:        result.Duration.Milliseconds(),
	}

	if result.FinalState != nil {
		v.TransportState = result.FinalState.TransportState
		v.TrackURI = result.FinalState.TrackURI
		v.DataSource = result.FinalState.Source
	}
	if result.FailureReason != "" {
		v.FailureReason = result.FailureReason
		v.FailureMessage = result.FailureMessage
	}

	return v
}

// resolveMemberIP resolves a scene member to an IP address.
func (e *Executor) resolveMemberIP(member SceneMember) (string, error) {
	// Try UDN first (primary identifier)
	ip, err := e.deviceService.ResolveDeviceIP(member.UDN)
	if err == nil && ip != "" {
		return ip, nil
	}

	// Fallback to room_name if UDN resolution failed
	if member.RoomName != "" {
		ip, err = e.deviceService.ResolveDeviceIP(member.RoomName)
		if err == nil && ip != "" {
			e.logger.Printf("Resolved device via room_name fallback: %s -> %s", member.RoomName, ip)
			return ip, nil
		}
	}

	return "", fmt.Errorf("could not resolve device IP for %s (room: %s)", member.UDN, member.RoomName)
}

// updateStep updates a step's status in the execution record.
func (e *Executor) updateStep(execID, stepName string, status StepStatus, errPtr *error, details map[string]any) {
	now := time.Now().UTC()
	update := StepUpdate{
		Status:  &status,
		Details: details,
	}

	if status == StepStatusRunning {
		update.StartedAt = &now
	}
	if status == StepStatusCompleted || status == StepStatusFailed {
		update.EndedAt = &now
	}
	if errPtr != nil && *errPtr != nil {
		errStr := (*errPtr).Error()
		update.Error = &errStr
	}

	if err := e.execRepo.UpdateStep(execID, stepName, update); err != nil {
		e.logger.Printf("Failed to update step %s: %v", stepName, err)
	}
}

// failExecution marks an execution as failed.
func (e *Executor) failExecution(execution *SceneExecution, err error) (*SceneExecution, error) {
	errMsg := err.Error()
	if completeErr := e.execRepo.Complete(execution.SceneExecutionID, ExecutionStatusFailed, nil, &errMsg); completeErr != nil {
		e.logger.Printf("Failed to mark execution as failed: %v", completeErr)
	}
	updated, _ := e.execRepo.GetByID(execution.SceneExecutionID)
	if updated != nil {
		return updated, err
	}
	return execution, err
}

// isArcBeamRay checks if a device model is an Arc, Beam, or Ray.
func isArcBeamRay(model string) bool {
	model = strings.ToLower(model)
	return strings.Contains(model, "arc") ||
		strings.Contains(model, "beam") ||
		strings.Contains(model, "ray") ||
		strings.Contains(model, "playbar") ||
		strings.Contains(model, "playbase")
}
