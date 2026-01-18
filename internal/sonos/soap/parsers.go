package soap

import (
	"bytes"
	"encoding/xml"
	"strconv"
	"strings"
)

func parseTextValue(payload []byte, element string) string {
	decoder := xml.NewDecoder(bytes.NewReader(payload))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch se := tok.(type) {
		case xml.StartElement:
			if se.Name.Local == element {
				var value string
				if err := decoder.DecodeElement(&value, &se); err == nil {
					return strings.TrimSpace(value)
				}
			}
		}
	}
	return ""
}

func parseTransportInfo(payload []byte) TransportInfo {
	return TransportInfo{
		CurrentTransportState:  parseTextValue(payload, "CurrentTransportState"),
		CurrentTransportStatus: parseTextValue(payload, "CurrentTransportStatus"),
		CurrentSpeed:           parseTextValue(payload, "CurrentSpeed"),
	}
}

func parsePositionInfo(payload []byte) PositionInfo {
	trackStr := parseTextValue(payload, "Track")
	track, _ := strconv.Atoi(trackStr)

	return PositionInfo{
		Track:         track,
		TrackDuration: parseTextValue(payload, "TrackDuration"),
		TrackMetaData: parseTextValue(payload, "TrackMetaData"),
		TrackURI:      parseTextValue(payload, "TrackURI"),
		RelTime:       parseTextValue(payload, "RelTime"),
		AbsTime:       parseTextValue(payload, "AbsTime"),
	}
}

func parseMediaInfo(payload []byte) MediaInfo {
	nrTracksStr := parseTextValue(payload, "NrTracks")
	nrTracks, _ := strconv.Atoi(nrTracksStr)

	return MediaInfo{
		NrTracks:           nrTracks,
		MediaDuration:      parseTextValue(payload, "MediaDuration"),
		CurrentURI:         parseTextValue(payload, "CurrentURI"),
		CurrentURIMetaData: parseTextValue(payload, "CurrentURIMetaData"),
	}
}

func parseVolume(payload []byte) VolumeInfo {
	volStr := parseTextValue(payload, "CurrentVolume")
	vol, _ := strconv.Atoi(volStr)
	return VolumeInfo{CurrentVolume: vol}
}

func parseMute(payload []byte) MuteInfo {
	muteStr := parseTextValue(payload, "CurrentMute")
	return MuteInfo{CurrentMute: muteStr == "1" || strings.EqualFold(muteStr, "true")}
}

func parseDeviceUUID(payload []byte) string {
	return parseTextValue(payload, "CurrentUUID")
}
