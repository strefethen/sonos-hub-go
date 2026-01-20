package scene

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/devices"
	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

// Executor orchestrates scene execution.
type Executor struct {
	logger        *log.Logger
	execRepo      *ExecutionsRepository
	lock          *CoordinatorLock
	preflight     *PreFlightChecker
	deviceService *devices.Service
	soapClient    *soap.Client
	timeout       time.Duration
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
		logger:        logger,
		execRepo:      execRepo,
		lock:          lock,
		preflight:     preflight,
		deviceService: deviceService,
		soapClient:    soapClient,
		timeout:       timeout,
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

	// Step 6: Start playback
	e.updateStep(execution.SceneExecutionID, "start_playback", StepStatusRunning, nil, nil)
	if err := e.startPlayback(coordinatorIP, options); err != nil {
		e.updateStep(execution.SceneExecutionID, "start_playback", StepStatusFailed, &err, nil)
		return e.failExecution(execution, err)
	}
	e.updateStep(execution.SceneExecutionID, "start_playback", StepStatusCompleted, nil, nil)

	// Step 7: Verify playback
	e.updateStep(execution.SceneExecutionID, "verify_playback", StepStatusRunning, nil, nil)
	verification := e.verifyPlayback(coordinatorIP)
	e.updateStep(execution.SceneExecutionID, "verify_playback", StepStatusCompleted, nil, map[string]any{
		"playback_confirmed": verification.PlaybackConfirmed,
		"transport_state":    verification.TransportState,
	})

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

// startPlayback starts playback on the coordinator.
func (e *Executor) startPlayback(coordinatorIP string, options ExecuteOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	// If music content is provided, set it up first
	if options.MusicContent != nil {
		if options.MusicContent.URI != "" {
			// Clear queue first if replacing
			if options.QueueMode == QueueModeReplaceAndPlay {
				if err := e.clearQueueWithRetry(ctx, coordinatorIP); err != nil {
					e.logger.Printf("Failed to clear queue: %v", err)
				}
			}

			// Set the URI
			if err := e.soapClient.SetAVTransportURI(ctx, coordinatorIP, options.MusicContent.URI, options.MusicContent.Metadata); err != nil {
				return fmt.Errorf("failed to set transport URI: %w", err)
			}
		}
	} else if options.FavoriteID != "" {
		// Legacy: play favorite by ID (would need to resolve favorite URI)
		e.logger.Printf("Playing favorite %s (legacy mode)", options.FavoriteID)
	}

	// Send play command
	if err := e.soapClient.Play(ctx, coordinatorIP); err != nil {
		return fmt.Errorf("failed to start playback: %w", err)
	}

	return nil
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

// verifyPlayback checks that playback is active.
func (e *Executor) verifyPlayback(coordinatorIP string) Verification {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	transportInfo, err := e.soapClient.GetTransportInfo(ctx, coordinatorIP)
	if err != nil {
		return Verification{
			PlaybackConfirmed:       false,
			VerificationUnavailable: true,
			CheckedAt:               time.Now().UTC(),
		}
	}

	positionInfo, _ := e.soapClient.GetPositionInfo(ctx, coordinatorIP)

	return Verification{
		PlaybackConfirmed: transportInfo.CurrentTransportState == "PLAYING",
		TransportState:    transportInfo.CurrentTransportState,
		TrackURI:          positionInfo.TrackURI,
		CheckedAt:         time.Now().UTC(),
	}
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
