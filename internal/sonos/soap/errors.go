package soap

import "fmt"

// SonosRejectedError represents a UPnP/SOAP error response from a device.
type SonosRejectedError struct {
	Action      string
	Code        string
	Description string
}

func (e *SonosRejectedError) Error() string {
	if e.Description == "" {
		return fmt.Sprintf("sonos action %s rejected: code %s", e.Action, e.Code)
	}
	return fmt.Sprintf("sonos action %s rejected: code %s (%s)", e.Action, e.Code, e.Description)
}

// SonosTimeoutError indicates a request timed out.
type SonosTimeoutError struct {
	Action string
}

func (e *SonosTimeoutError) Error() string {
	return fmt.Sprintf("sonos action %s timed out", e.Action)
}

// SonosUnreachableError indicates the device could not be reached.
type SonosUnreachableError struct {
	Action string
	Err    error
}

func (e *SonosUnreachableError) Error() string {
	return fmt.Sprintf("sonos action %s unreachable: %v", e.Action, e.Err)
}

func (e *SonosUnreachableError) Unwrap() error {
	return e.Err
}
