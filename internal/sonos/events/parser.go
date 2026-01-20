package events

import (
	"bytes"
	"encoding/xml"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// UPnP propertyset structure
type propertyset struct {
	XMLName    xml.Name   `xml:"propertyset"`
	Properties []property `xml:"property"`
}

type property struct {
	LastChange string `xml:"LastChange"`
	// Other properties can be added as needed
}

// AVTransport LastChange event structure
type avTransportEvent struct {
	XMLName    xml.Name         `xml:"Event"`
	InstanceID avTransportInstance `xml:"InstanceID"`
}

type avTransportInstance struct {
	Val                    string `xml:"val,attr"`
	TransportState         attrVal `xml:"TransportState"`
	CurrentTransportActions attrVal `xml:"CurrentTransportActions"`
	TransportStatus        attrVal `xml:"TransportStatus"`
	TransportPlaySpeed     attrVal `xml:"TransportPlaySpeed"`
	CurrentTrack           attrVal `xml:"CurrentTrack"`
	CurrentTrackURI        attrVal `xml:"CurrentTrackURI"`
	CurrentTrackDuration   attrVal `xml:"CurrentTrackDuration"`
	CurrentTrackMetaData   attrVal `xml:"CurrentTrackMetaData"`
	AVTransportURI         attrVal `xml:"AVTransportURI"`
	AVTransportURIMetaData attrVal `xml:"AVTransportURIMetaData"`
	RelTime                attrVal `xml:"RelativeTimePosition"`
}

type attrVal struct {
	Val string `xml:"val,attr"`
}

// RenderingControl LastChange event structure
type renderingControlEvent struct {
	XMLName    xml.Name               `xml:"Event"`
	InstanceID renderingControlInstance `xml:"InstanceID"`
}

type renderingControlInstance struct {
	Val     string         `xml:"val,attr"`
	Volume  channelAttrVal `xml:"Volume"`
	Mute    channelAttrVal `xml:"Mute"`
}

type channelAttrVal struct {
	Channel string `xml:"channel,attr"`
	Val     string `xml:"val,attr"`
}

// ParseNotifyBody parses a UPnP NOTIFY event body.
// Sonos events use double-encoded XML in the LastChange property.
func ParseNotifyBody(body []byte, serviceType ServiceType) (*NotifyEvent, error) {
	event := &NotifyEvent{
		ServiceType: serviceType,
		Properties:  make(map[string]string),
		RawBody:     body,
	}

	// Parse the outer propertyset envelope
	var ps propertyset
	if err := xml.Unmarshal(body, &ps); err != nil {
		return nil, err
	}

	// Extract and parse the LastChange property
	for _, prop := range ps.Properties {
		if prop.LastChange != "" {
			// LastChange contains XML-escaped content that needs to be unescaped
			unescaped := html.UnescapeString(prop.LastChange)

			switch serviceType {
			case ServiceAVTransport:
				avEvent, err := parseAVTransportLastChange(unescaped)
				if err == nil {
					mergeAVTransportProperties(event.Properties, avEvent)
				}
			case ServiceRenderingControl:
				rcEvent, err := parseRenderingControlLastChange(unescaped)
				if err == nil {
					mergeRenderingControlProperties(event.Properties, rcEvent)
				}
			case ServiceZoneGroupTopology:
				event.Properties["ZoneGroupState"] = unescaped
			}
		}
	}

	return event, nil
}

// parseAVTransportLastChange parses the unescaped AVTransport LastChange XML.
func parseAVTransportLastChange(xmlContent string) (*AVTransportEvent, error) {
	var evt avTransportEvent
	if err := xml.Unmarshal([]byte(xmlContent), &evt); err != nil {
		return nil, err
	}

	return &AVTransportEvent{
		TransportState:         evt.InstanceID.TransportState.Val,
		TransportStatus:        evt.InstanceID.TransportStatus.Val,
		CurrentTrackURI:        evt.InstanceID.CurrentTrackURI.Val,
		CurrentTrackMetaData:   evt.InstanceID.CurrentTrackMetaData.Val,
		TrackDuration:          evt.InstanceID.CurrentTrackDuration.Val,
		RelTime:                evt.InstanceID.RelTime.Val,
		AVTransportURI:         evt.InstanceID.AVTransportURI.Val,
		AVTransportURIMetaData: evt.InstanceID.AVTransportURIMetaData.Val,
	}, nil
}

// parseRenderingControlLastChange parses the unescaped RenderingControl LastChange XML.
func parseRenderingControlLastChange(xmlContent string) (*RenderingControlEvent, error) {
	var evt renderingControlEvent
	if err := xml.Unmarshal([]byte(xmlContent), &evt); err != nil {
		return nil, err
	}

	event := &RenderingControlEvent{}

	// Parse volume (only use Master channel)
	if evt.InstanceID.Volume.Channel == "Master" || evt.InstanceID.Volume.Channel == "" {
		if vol, err := strconv.Atoi(evt.InstanceID.Volume.Val); err == nil {
			event.Volume = vol
		}
	}

	// Parse mute
	if evt.InstanceID.Mute.Channel == "Master" || evt.InstanceID.Mute.Channel == "" {
		event.Muted = evt.InstanceID.Mute.Val == "1"
	}

	return event, nil
}

// mergeAVTransportProperties adds AVTransport event data to the properties map.
func mergeAVTransportProperties(props map[string]string, evt *AVTransportEvent) {
	if evt.TransportState != "" {
		props["TransportState"] = evt.TransportState
	}
	if evt.TransportStatus != "" {
		props["TransportStatus"] = evt.TransportStatus
	}
	if evt.CurrentTrackURI != "" {
		props["CurrentTrackURI"] = evt.CurrentTrackURI
	}
	if evt.CurrentTrackMetaData != "" {
		props["CurrentTrackMetaData"] = evt.CurrentTrackMetaData
	}
	if evt.TrackDuration != "" {
		props["TrackDuration"] = evt.TrackDuration
	}
	if evt.RelTime != "" {
		props["RelTime"] = evt.RelTime
	}
	if evt.AVTransportURI != "" {
		props["AVTransportURI"] = evt.AVTransportURI
	}
	if evt.AVTransportURIMetaData != "" {
		props["AVTransportURIMetaData"] = evt.AVTransportURIMetaData
	}
}

// mergeRenderingControlProperties adds RenderingControl event data to the properties map.
func mergeRenderingControlProperties(props map[string]string, evt *RenderingControlEvent) {
	props["Volume"] = strconv.Itoa(evt.Volume)
	if evt.Muted {
		props["Mute"] = "1"
	} else {
		props["Mute"] = "0"
	}
}

// ParseSID extracts the subscription ID from a SUBSCRIBE response header.
func ParseSID(sidHeader string) string {
	// SID format: uuid:RINCON_xxx_sub0000000001
	if strings.HasPrefix(sidHeader, "uuid:") {
		return sidHeader
	}
	return sidHeader
}

// ParseTimeout extracts the timeout value from a SUBSCRIBE response header.
// Returns timeout in seconds.
func ParseTimeout(timeoutHeader string) int {
	// Timeout format: Second-3600 or infinite
	if timeoutHeader == "infinite" {
		// Return 24 hours for infinite subscriptions to avoid renewal buffer
		// calculation going negative (0 - 60 = -60 seconds = immediate expiry loop)
		return 86400
	}

	timeoutHeader = strings.TrimPrefix(timeoutHeader, "Second-")
	if timeout, err := strconv.Atoi(timeoutHeader); err == nil {
		return timeout
	}
	return 3600 // Default to 1 hour
}

// ParseSEQ extracts the sequence number from a NOTIFY header.
func ParseSEQ(seqHeader string) int {
	if seq, err := strconv.Atoi(seqHeader); err == nil {
		return seq
	}
	return 0
}

// InferServiceTypeFromPath infers the service type from the callback path.
func InferServiceTypeFromPath(path string) ServiceType {
	switch {
	case strings.Contains(path, "avtransport"):
		return ServiceAVTransport
	case strings.Contains(path, "renderingcontrol"):
		return ServiceRenderingControl
	case strings.Contains(path, "topology"):
		return ServiceZoneGroupTopology
	default:
		return ServiceAVTransport // Default
	}
}

// volumeRegex matches Volume elements in RenderingControl events
var volumeRegex = regexp.MustCompile(`<Volume[^>]*channel="Master"[^>]*val="(\d+)"`)
var muteRegex = regexp.MustCompile(`<Mute[^>]*channel="Master"[^>]*val="([01])"`)

// ParseRenderingControlFast provides a fast regex-based parser for RenderingControl events.
// Use this when performance is critical and full XML parsing isn't needed.
func ParseRenderingControlFast(body []byte) *RenderingControlEvent {
	event := &RenderingControlEvent{}

	// Unescape the body first
	unescaped := bytes.ReplaceAll(body, []byte("&lt;"), []byte("<"))
	unescaped = bytes.ReplaceAll(unescaped, []byte("&gt;"), []byte(">"))
	unescaped = bytes.ReplaceAll(unescaped, []byte("&amp;"), []byte("&"))
	unescaped = bytes.ReplaceAll(unescaped, []byte("&quot;"), []byte("\""))

	// Extract volume
	if matches := volumeRegex.FindSubmatch(unescaped); len(matches) > 1 {
		if vol, err := strconv.Atoi(string(matches[1])); err == nil {
			event.Volume = vol
		}
	}

	// Extract mute
	if matches := muteRegex.FindSubmatch(unescaped); len(matches) > 1 {
		event.Muted = string(matches[1]) == "1"
	}

	return event
}

// transportStateRegex matches TransportState in AVTransport events
var transportStateRegex = regexp.MustCompile(`<TransportState[^>]*val="([^"]+)"`)

// ParseTransportStateFast provides a fast regex-based parser for transport state.
func ParseTransportStateFast(body []byte) string {
	// Unescape the body first
	unescaped := bytes.ReplaceAll(body, []byte("&lt;"), []byte("<"))
	unescaped = bytes.ReplaceAll(unescaped, []byte("&gt;"), []byte(">"))

	if matches := transportStateRegex.FindSubmatch(unescaped); len(matches) > 1 {
		return string(matches[1])
	}
	return ""
}
