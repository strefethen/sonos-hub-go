package discovery

import (
	"encoding/xml"
	"strings"
)

type DeviceDescription struct {
	ModelName       string
	ModelNumber     string
	RoomName        string
	SerialNumber    string
	SoftwareVersion string
	HardwareVersion string
	SupportsAirPlay bool
	UDN             string
}

type ZoneInfo struct {
	RoomName      string
	ZoneGroupID   string
	Coordinator   string
	IsCoordinator bool
}

func ParseDeviceDescription(xmlPayload []byte) (*DeviceDescription, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(xmlPayload)))
	var desc DeviceDescription

	var friendlyName string
	var udnRaw string
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch se := tok.(type) {
		case xml.StartElement:
			switch se.Name.Local {
			case "friendlyName":
				var value string
				if err := decoder.DecodeElement(&value, &se); err == nil {
					friendlyName = strings.TrimSpace(value)
				}
			case "modelName":
				var value string
				if err := decoder.DecodeElement(&value, &se); err == nil {
					desc.ModelName = strings.TrimSpace(value)
				}
			case "modelNumber":
				var value string
				if err := decoder.DecodeElement(&value, &se); err == nil {
					desc.ModelNumber = strings.TrimSpace(value)
				}
			case "serialNum":
				var value string
				if err := decoder.DecodeElement(&value, &se); err == nil {
					desc.SerialNumber = strings.TrimSpace(value)
				}
			case "softwareVersion":
				var value string
				if err := decoder.DecodeElement(&value, &se); err == nil {
					desc.SoftwareVersion = strings.TrimSpace(value)
				}
			case "hardwareVersion":
				var value string
				if err := decoder.DecodeElement(&value, &se); err == nil {
					desc.HardwareVersion = strings.TrimSpace(value)
				}
			case "UDN":
				// Only take the FIRST UDN (root device).
				// Device description XML has multiple UDNs: root (RINCON_xxx),
				// MediaServer (RINCON_xxx_MS), MediaRenderer (RINCON_xxx_MR).
				// Node.js gets the root UDN, so we do the same.
				if udnRaw == "" {
					var value string
					if err := decoder.DecodeElement(&value, &se); err == nil {
						udnRaw = strings.TrimSpace(value)
					}
				}
			}
		}
	}

	if friendlyName != "" {
		desc.RoomName = parseRoomName(friendlyName)
	}

	if udnRaw != "" {
		desc.UDN = strings.TrimPrefix(udnRaw, "uuid:")
	}

	desc.SupportsAirPlay = supportsAirPlayModel(desc.ModelNumber)

	return &desc, nil
}

func parseRoomName(friendlyName string) string {
	if friendlyName == "" {
		return ""
	}
	parts := strings.SplitN(friendlyName, "-", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0])
	}
	return strings.TrimSpace(friendlyName)
}

func supportsAirPlayModel(modelNumber string) bool {
	airPlayModels := map[string]struct{}{
		"S18": {},
		"S14": {},
		"S38": {},
		"S21": {},
		"S27": {},
		"S17": {},
		"S23": {},
		"S36": {},
		"S37": {},
		"S6":  {},
		"S31": {},
		"S24": {},
		"S3":  {},
	}
	_, ok := airPlayModels[modelNumber]
	return ok
}

func ParseZoneInfo(xmlPayload []byte) (*ZoneInfo, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(xmlPayload)))
	var info ZoneInfo

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch se := tok.(type) {
		case xml.StartElement:
			switch se.Name.Local {
			case "ZoneName":
				var value string
				if err := decoder.DecodeElement(&value, &se); err == nil {
					info.RoomName = strings.TrimSpace(value)
				}
			case "ZoneGroupID":
				var value string
				if err := decoder.DecodeElement(&value, &se); err == nil {
					info.ZoneGroupID = strings.TrimSpace(value)
				}
			case "Coordinator":
				var value string
				if err := decoder.DecodeElement(&value, &se); err == nil {
					info.Coordinator = strings.TrimSpace(value)
				}
			case "IsCoordinator":
				var value string
				if err := decoder.DecodeElement(&value, &se); err == nil {
					info.IsCoordinator = value == "1" || strings.EqualFold(value, "true")
				}
			}
		}
	}

	if info.RoomName == "" {
		return nil, nil
	}
	return &info, nil
}
