package devices

import "log"

func mergeTopologies(newTopo DeviceTopology, existing *DeviceTopology) DeviceTopology {
	if existing == nil {
		updated := make([]LogicalDevice, 0, len(newTopo.Devices))
		for _, device := range newTopo.Devices {
			device.Health = DeviceHealthOK
			device.MissedScans = 0
			device.PhysicalDevices = resetPhysicalHealth(device.PhysicalDevices, DeviceHealthOK, 0)
			updated = append(updated, device)
		}
		newTopo.Devices = updated
		return newTopo
	}

	newDeviceByUDN := make(map[string]LogicalDevice)
	for _, device := range newTopo.Devices {
		if udn := primaryUDN(device); udn != "" {
			newDeviceByUDN[udn] = device
		}
	}

	existingDeviceByUDN := make(map[string]LogicalDevice)
	for _, device := range existing.Devices {
		if udn := primaryUDN(device); udn != "" {
			existingDeviceByUDN[udn] = device
		}
	}

	merged := make([]LogicalDevice, 0, len(newTopo.Devices))
	processed := make(map[string]struct{})

	for _, device := range newTopo.Devices {
		udn := primaryUDN(device)
		if udn == "" {
			continue
		}
		processed[udn] = struct{}{}

		existingDevice, ok := existingDeviceByUDN[udn]
		if ok {
			device.DeviceID = existingDevice.DeviceID
		}
		device.Health = DeviceHealthOK
		device.MissedScans = 0
		device.PhysicalDevices = resetPhysicalHealth(device.PhysicalDevices, DeviceHealthOK, 0)
		merged = append(merged, device)
	}

	for _, device := range existing.Devices {
		udn := primaryUDN(device)
		if udn == "" {
			continue
		}
		if _, ok := processed[udn]; ok {
			continue
		}

		missed := device.MissedScans + 1
		health := computeHealth(missed)

		if missed >= RemovalThreshold {
			log.Printf("Removing device after missed scans: %s (%s)", device.DeviceID, device.RoomName)
			continue
		}

		device.Health = health
		device.MissedScans = missed
		device.PhysicalDevices = resetPhysicalHealth(device.PhysicalDevices, health, missed)
		merged = append(merged, device)
	}

	return DeviceTopology{
		Devices:           merged,
		HomeTheaterGroups: newTopo.HomeTheaterGroups,
		StereoPairs:       newTopo.StereoPairs,
		UpdatedAt:         newTopo.UpdatedAt,
	}
}

func computeHealth(missedScans int) DeviceHealthStatus {
	if missedScans >= OfflineThreshold {
		return DeviceHealthOffline
	}
	if missedScans >= DegradedThreshold {
		return DeviceHealthDegraded
	}
	return DeviceHealthOK
}

func primaryUDN(device LogicalDevice) string {
	if len(device.PhysicalDevices) == 0 {
		return ""
	}
	return device.PhysicalDevices[0].UDN
}

func resetPhysicalHealth(devices []PhysicalDevice, health DeviceHealthStatus, missed int) []PhysicalDevice {
	updated := make([]PhysicalDevice, 0, len(devices))
	for _, device := range devices {
		device.Health = health
		device.MissedScans = missed
		updated = append(updated, device)
	}
	return updated
}
