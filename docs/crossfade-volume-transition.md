# Scene Execution: Crossfade Volume Transition

## Problem

Currently, scene execution:
1. Sets volume instantly on old content
2. Starts new content

This creates a jarring transition. The user wants a smooth crossfade:
1. Fade old content OUT (ramp down)
2. Start new content (at low/zero volume)
3. Fade new content IN (ramp up to target)

## Technical Context

**Sonos Capabilities:**
- ❌ No native `RampToVolume` SOAP command
- ✅ Manual ramping already implemented via `executeVolumeRamp()` in `routes.go`
- ✅ Supports 50ms minimum step intervals
- ✅ Easing curves: `linear`, `ease-in`, `ease-out`

**Existing Code:**
- `VolumeRamp` type defined in `scene/types.go` but not used
- `executeVolumeRamp()` in `sonos/routes.go` (lines 1330-1413) handles the actual ramping

## Solution

Implement crossfade in scene execution with configurable fade duration:

**New Execution Order:**
1. Determine Coordinator
2. Acquire Lock
3. Ensure Group
4. Pre-Flight Check
5. **Fade OUT** (ramp current volume → 0)
6. **Start Playback** (new content begins at 0 volume)
7. **Fade IN** (ramp 0 → target volume)
8. Verify Playback

## Files to Modify

| File | Change |
|------|--------|
| `internal/scene/executor.go` | Add `fadeOut()` and `fadeIn()` methods, reorder steps |
| `internal/scene/types.go` | Ensure `VolumeRamp` is used, possibly add crossfade duration |

## Implementation

### 1. Add Ramp Functions to Executor

In `executor.go`, add helper methods that use the existing ramping algorithm:

```go
// fadeOut ramps all members to volume 0 before content switch
func (e *Executor) fadeOut(scene *Scene, durationMs int) {
    for _, member := range scene.Members {
        memberIP, err := e.resolveMemberIP(member)
        if err != nil {
            continue
        }

        // Get current volume
        ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
        volInfo, err := e.soapClient.GetVolume(ctx, memberIP)
        cancel()
        if err != nil {
            continue
        }

        // Ramp down to 0
        e.rampVolume(memberIP, volInfo.CurrentVolume, 0, durationMs, "ease-out")
    }
}

// fadeIn ramps all members from 0 to their target volume
func (e *Executor) fadeIn(scene *Scene, durationMs int) {
    for _, member := range scene.Members {
        if member.TargetVolume == nil {
            continue
        }
        memberIP, err := e.resolveMemberIP(member)
        if err != nil {
            continue
        }

        // Ramp up from 0 to target
        e.rampVolume(memberIP, 0, *member.TargetVolume, durationMs, "ease-in")
    }
}

// rampVolume performs gradual volume change with easing
func (e *Executor) rampVolume(ip string, startLevel, targetLevel, durationMs int, curve string) {
    stepCount := int(math.Max(1, float64(durationMs/50)))
    levelDiff := float64(targetLevel - startLevel)
    stepDelay := time.Duration(float64(durationMs)/float64(stepCount)) * time.Millisecond

    for step := 1; step <= stepCount; step++ {
        progress := float64(step) / float64(stepCount)

        // Apply easing curve
        switch curve {
        case "ease-in":
            progress = progress * progress
        case "ease-out":
            progress = 1 - math.Pow(1-progress, 2)
        }

        newLevel := int(math.Round(float64(startLevel) + levelDiff*progress))

        ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
        e.soapClient.SetVolume(ctx, ip, newLevel)
        cancel()

        time.Sleep(stepDelay)
    }

    // Final set to ensure exact target
    ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
    e.soapClient.SetVolume(ctx, ip, targetLevel)
    cancel()
}
```

### 2. Update Execute() Method

Replace the current volume + playback order:

```go
// Step 4: Pre-flight check
e.updateStep(execution.SceneExecutionID, "pre_flight_check", StepStatusRunning, nil, nil)
if err := e.runPreFlightWithRecovery(coordinatorIP, coordinator.RoomName, options.TVPolicy); err != nil {
    e.updateStep(execution.SceneExecutionID, "pre_flight_check", StepStatusFailed, &err, nil)
    return e.failExecution(execution, err)
}
e.updateStep(execution.SceneExecutionID, "pre_flight_check", StepStatusCompleted, nil, nil)

// Step 5: Fade OUT (ramp old content down)
fadeDurationMs := 2000 // 2 seconds default, could come from scene.VolumeRamp.DurationMs
e.updateStep(execution.SceneExecutionID, "fade_out", StepStatusRunning, nil, nil)
e.fadeOut(scene, fadeDurationMs)
e.updateStep(execution.SceneExecutionID, "fade_out", StepStatusCompleted, nil, map[string]any{
    "duration_ms": fadeDurationMs,
})

// Step 6: Start playback (content starts at 0 volume)
e.updateStep(execution.SceneExecutionID, "start_playback", StepStatusRunning, nil, nil)
expectedContent, err := e.startPlayback(coordinatorIP, coordinatorUDN, options)
if err != nil {
    e.updateStep(execution.SceneExecutionID, "start_playback", StepStatusFailed, &err, nil)
    return e.failExecution(execution, err)
}
// ... existing startPlaybackDetails code ...
e.updateStep(execution.SceneExecutionID, "start_playback", StepStatusCompleted, nil, startPlaybackDetails)

// Step 7: Fade IN (ramp new content up to target)
e.updateStep(execution.SceneExecutionID, "fade_in", StepStatusRunning, nil, nil)
e.fadeIn(scene, fadeDurationMs)
e.updateStep(execution.SceneExecutionID, "fade_in", StepStatusCompleted, nil, map[string]any{
    "duration_ms": fadeDurationMs,
})

// Step 8: Verify playback (renumbered from step 7)
// ... existing verification code ...
```

### 3. Use VolumeRamp Configuration

The `Scene` already has a `VolumeRamp` field. Wire it in:

```go
// Get fade duration from scene config, or use default
fadeDurationMs := 2000
if scene.VolumeRamp != nil && scene.VolumeRamp.Enabled && scene.VolumeRamp.DurationMs != nil {
    fadeDurationMs = *scene.VolumeRamp.DurationMs
}

// Use configured curve or default
curve := "linear"
if scene.VolumeRamp != nil && scene.VolumeRamp.Curve != "" {
    curve = scene.VolumeRamp.Curve
}
```

## Timing Analysis

With 2-second fades:
```
T=0.0s: Start fade OUT (volume 50 → 0, 2s)
T=2.0s: Fade OUT complete, start new playback
T=2.1s: Start fade IN (volume 0 → 50, 2s)
T=4.1s: Fade IN complete, verify playback
Total: ~4.5 seconds for smooth crossfade
```

## Considerations

1. **Skip fade if no previous content**: If device is already stopped/idle, skip fade-out
2. **Parallel fade for multi-device**: Use goroutines for concurrent ramping
3. **Error handling**: If fade-out fails, still try to start playback (don't abort)
4. **VolumeRamp disabled**: If `scene.VolumeRamp.Enabled == false`, use instant volume (current behavior)

## Verification

1. Start backend: `set -a && source .env && set +a && air`
2. Play music on a Sonos speaker
3. Execute a scene targeting that speaker
4. Observe:
   - Old music fades out smoothly
   - Brief silence/low volume moment
   - New music fades in smoothly
5. Test with VolumeRamp enabled/disabled in scene config
