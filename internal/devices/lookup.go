package devices

import (
	"log"
	"strings"
)

func findDevice(devices []LogicalDevice, logger *log.Logger, deviceID string) *LogicalDevice {
	for _, device := range devices {
		if device.DeviceID == deviceID {
			copy := device
			return &copy
		}
	}

	for _, device := range devices {
		if len(device.PhysicalDevices) == 0 {
			continue
		}
		if device.PhysicalDevices[0].UDN == deviceID {
			copy := device
			return &copy
		}
	}

	for _, device := range devices {
		if strings.EqualFold(device.RoomName, deviceID) {
			logger.Printf("Device found by room name fallback: requested=%s found=%s", deviceID, device.DeviceID)
			copy := device
			return &copy
		}
	}

	return nil
}
