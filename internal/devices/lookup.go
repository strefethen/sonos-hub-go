package devices

import (
	"log"
	"strings"
)

// findDevice looks up a device by UDN or room name.
// The lookup order is: UDN → room_name (case-insensitive fallback)
func findDevice(devices []LogicalDevice, logger *log.Logger, identifier string) *LogicalDevice {
	// First, try to find by UDN (primary identifier)
	for _, device := range devices {
		if device.UDN == identifier {
			copy := device
			return &copy
		}
	}

	// Fallback: try room name (case-insensitive)
	for _, device := range devices {
		if strings.EqualFold(device.RoomName, identifier) {
			logger.Printf("Device found by room name fallback: requested=%s found=%s", identifier, device.UDN)
			copy := device
			return &copy
		}
	}

	return nil
}
