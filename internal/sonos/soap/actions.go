package soap

import (
	"bytes"
	"context"
	"encoding/xml"
	"strconv"
	"strings"
	"time"
)

// Transport Actions
func (c *Client) GetTransportInfo(ctx context.Context, ip string) (TransportInfo, error) {
	payload, err := c.ExecuteAction(ctx, ip, ServiceAVTransport, "GetTransportInfo", map[string]string{
		"InstanceID": "0",
	})
	if err != nil {
		return TransportInfo{}, err
	}
	return parseTransportInfo(payload), nil
}

func (c *Client) GetPositionInfo(ctx context.Context, ip string) (PositionInfo, error) {
	payload, err := c.ExecuteAction(ctx, ip, ServiceAVTransport, "GetPositionInfo", map[string]string{
		"InstanceID": "0",
	})
	if err != nil {
		return PositionInfo{}, err
	}
	return parsePositionInfo(payload), nil
}

func (c *Client) GetMediaInfo(ctx context.Context, ip string) (MediaInfo, error) {
	payload, err := c.ExecuteAction(ctx, ip, ServiceAVTransport, "GetMediaInfo", map[string]string{
		"InstanceID": "0",
	})
	if err != nil {
		return MediaInfo{}, err
	}
	return parseMediaInfo(payload), nil
}

func (c *Client) Play(ctx context.Context, ip string) error {
	_, err := c.ExecuteAction(ctx, ip, ServiceAVTransport, "Play", map[string]string{
		"InstanceID": "0",
		"Speed":      "1",
	})
	return err
}

func (c *Client) Pause(ctx context.Context, ip string) error {
	_, err := c.ExecuteAction(ctx, ip, ServiceAVTransport, "Pause", map[string]string{
		"InstanceID": "0",
	})
	return err
}

func (c *Client) Stop(ctx context.Context, ip string) error {
	_, err := c.ExecuteAction(ctx, ip, ServiceAVTransport, "Stop", map[string]string{
		"InstanceID": "0",
	})
	return err
}

func (c *Client) Next(ctx context.Context, ip string) error {
	_, err := c.ExecuteAction(ctx, ip, ServiceAVTransport, "Next", map[string]string{
		"InstanceID": "0",
	})
	return err
}

func (c *Client) Previous(ctx context.Context, ip string) error {
	_, err := c.ExecuteAction(ctx, ip, ServiceAVTransport, "Previous", map[string]string{
		"InstanceID": "0",
	})
	return err
}

func (c *Client) SetAVTransportURI(ctx context.Context, ip, uri, metadata string) error {
	_, err := c.ExecuteAction(ctx, ip, ServiceAVTransport, "SetAVTransportURI", map[string]string{
		"InstanceID":         "0",
		"CurrentURI":         uri,
		"CurrentURIMetaData": metadata,
	})
	return err
}

func (c *Client) AddURIToQueue(ctx context.Context, ip, uri, metadata string, position int, enqueueNext bool) (int, error) {
	enqueueAsNext := "0"
	if enqueueNext {
		enqueueAsNext = "1"
	}
	payload, err := c.ExecuteAction(ctx, ip, ServiceAVTransport, "AddURIToQueue", map[string]string{
		"InstanceID":                      "0",
		"EnqueuedURI":                     uri,
		"EnqueuedURIMetaData":             metadata,
		"DesiredFirstTrackNumberEnqueued": strconv.Itoa(position),
		"EnqueueAsNext":                   enqueueAsNext,
	})
	if err != nil {
		return 0, err
	}

	trackStr := parseTextValue(payload, "FirstTrackNumberEnqueued")
	trackNum, _ := strconv.Atoi(trackStr)
	return trackNum, nil
}

func (c *Client) RemoveAllTracksFromQueue(ctx context.Context, ip string) error {
	_, err := c.ExecuteAction(ctx, ip, ServiceAVTransport, "RemoveAllTracksFromQueue", map[string]string{
		"InstanceID": "0",
	})
	return err
}

func (c *Client) Seek(ctx context.Context, ip, unit, target string) error {
	_, err := c.ExecuteAction(ctx, ip, ServiceAVTransport, "Seek", map[string]string{
		"InstanceID": "0",
		"Unit":       unit,
		"Target":     target,
	})
	return err
}

func (c *Client) BecomeCoordinatorOfStandaloneGroup(ctx context.Context, ip string) error {
	_, err := c.ExecuteAction(ctx, ip, ServiceAVTransport, "BecomeCoordinatorOfStandaloneGroup", map[string]string{
		"InstanceID": "0",
	})
	return err
}

// RenderingControl Actions
func (c *Client) GetVolume(ctx context.Context, ip string) (VolumeInfo, error) {
	payload, err := c.ExecuteAction(ctx, ip, ServiceRenderingControl, "GetVolume", map[string]string{
		"InstanceID": "0",
		"Channel":    "Master",
	})
	if err != nil {
		return VolumeInfo{}, err
	}
	return parseVolume(payload), nil
}

func (c *Client) SetVolume(ctx context.Context, ip string, level int) error {
	_, err := c.ExecuteAction(ctx, ip, ServiceRenderingControl, "SetVolume", map[string]string{
		"InstanceID":    "0",
		"Channel":       "Master",
		"DesiredVolume": strconv.Itoa(level),
	})
	return err
}

func (c *Client) GetMute(ctx context.Context, ip string) (MuteInfo, error) {
	payload, err := c.ExecuteAction(ctx, ip, ServiceRenderingControl, "GetMute", map[string]string{
		"InstanceID": "0",
		"Channel":    "Master",
	})
	if err != nil {
		return MuteInfo{}, err
	}
	return parseMute(payload), nil
}

func (c *Client) SetMute(ctx context.Context, ip string, mute bool) error {
	desired := "0"
	if mute {
		desired = "1"
	}
	_, err := c.ExecuteAction(ctx, ip, ServiceRenderingControl, "SetMute", map[string]string{
		"InstanceID":  "0",
		"Channel":     "Master",
		"DesiredMute": desired,
	})
	return err
}

// ZoneGroupTopology Actions
func (c *Client) GetZoneGroupState(ctx context.Context, ip string) (ZoneGroupState, error) {
	payload, err := c.ExecuteAction(ctx, ip, ServiceZoneGroupTopology, "GetZoneGroupState", map[string]string{})
	if err != nil {
		return ZoneGroupState{}, err
	}
	state := parseZoneGroupState(payload)
	return state, nil
}

// DeviceProperties Actions
func (c *Client) GetZoneAttributes(ctx context.Context, ip string) (ZoneAttributes, error) {
	payload, err := c.ExecuteAction(ctx, ip, ServiceDeviceProperties, "GetZoneAttributes", map[string]string{})
	if err != nil {
		return ZoneAttributes{}, err
	}
	return parseZoneAttributes(payload), nil
}

// AlarmClock Actions
func (c *Client) ListAlarms(ctx context.Context, ip string) (AlarmListResult, error) {
	payload, err := c.ExecuteAction(ctx, ip, ServiceAlarmClock, "ListAlarms", map[string]string{})
	if err != nil {
		return AlarmListResult{}, err
	}
	return parseAlarmList(payload), nil
}

// ContentDirectory Actions
func (c *Client) Browse(ctx context.Context, ip, objectID, browseFlag, filter string, startIndex, requestedCount int) (BrowseResult, error) {
	payload, err := c.ExecuteAction(ctx, ip, ServiceContentDirectory, "Browse", map[string]string{
		"ObjectID":       objectID,
		"BrowseFlag":     browseFlag,
		"Filter":         filter,
		"StartingIndex":  strconv.Itoa(startIndex),
		"RequestedCount": strconv.Itoa(requestedCount),
		"SortCriteria":   "",
	})
	if err != nil {
		return BrowseResult{}, err
	}
	return parseBrowseResult(payload), nil
}

// ZoneAttributes minimal response.
type ZoneAttributes struct {
	CurrentZoneName string
}

func parseZoneAttributes(payload []byte) ZoneAttributes {
	return ZoneAttributes{CurrentZoneName: parseTextValue(payload, "CurrentZoneName")}
}

// BrowseResult mirrors ContentDirectory Browse response (subset).
type BrowseResult struct {
	Result         string
	NumberReturned int
	TotalMatches   int
	UpdateID       int
	Items          []FavoriteItem
}

func parseBrowseResult(payload []byte) BrowseResult {
	result := BrowseResult{}
	result.Result = parseTextValue(payload, "Result")
	result.NumberReturned, _ = strconv.Atoi(parseTextValue(payload, "NumberReturned"))
	result.TotalMatches, _ = strconv.Atoi(parseTextValue(payload, "TotalMatches"))
	result.UpdateID, _ = strconv.Atoi(parseTextValue(payload, "UpdateID"))

	if result.Result == "" {
		return result
	}

	items := parseDidlFavorites([]byte(result.Result))
	result.Items = items
	return result
}

// parseZoneGroupState parses GetZoneGroupState response XML and returns minimal structure.
func parseZoneGroupState(payload []byte) ZoneGroupState {
	zoneXML := parseTextValue(payload, "ZoneGroupState")
	if zoneXML == "" {
		zoneXML = string(payload)
	}

	decoder := xml.NewDecoder(strings.NewReader(zoneXML))
	var state ZoneGroupState
	var currentGroup *ZoneGroup
	var coordinator string

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch se := tok.(type) {
		case xml.StartElement:
			switch se.Name.Local {
			case "ZoneGroup":
				group := ZoneGroup{}
				coordinator = ""
				for _, attr := range se.Attr {
					if attr.Name.Local == "ID" {
						group.ID = attr.Value
					}
					if attr.Name.Local == "Coordinator" {
						group.Coordinator = attr.Value
						coordinator = attr.Value
					}
				}
				state.Groups = append(state.Groups, group)
				currentGroup = &state.Groups[len(state.Groups)-1]
			case "ZoneGroupMember":
				if currentGroup == nil {
					continue
				}
				member := ZoneMember{
					IsVisible: true,
				}
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "UUID":
						member.UUID = attr.Value
					case "ZoneName":
						member.ZoneName = attr.Value
					case "Location":
						member.Location = attr.Value
					case "ChannelMapSet":
						member.ChannelMapSet = attr.Value
					case "Invisible":
						member.IsVisible = !(attr.Value == "true" || attr.Value == "1")
					}
				}
				if member.UUID != "" && member.UUID == coordinator {
					member.IsCoordinator = true
				}
				currentGroup.Members = append(currentGroup.Members, member)
			case "Satellite":
				if currentGroup == nil {
					continue
				}
				satellite := ZoneMember{}
				var htSatChan string
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "UUID":
						satellite.UUID = attr.Value
					case "ZoneName":
						satellite.ZoneName = attr.Value
					case "Location":
						satellite.Location = attr.Value
					case "ChannelMapSet":
						satellite.ChannelMapSet = attr.Value
					case "HTSatChanMapSet":
						htSatChan = attr.Value
					}
				}
				if strings.Contains(htSatChan, ":SW") {
					satellite.IsSubwoofer = true
				}
				if strings.Contains(htSatChan, ":LR") || strings.Contains(htSatChan, ":RR") {
					satellite.IsSatellite = true
				}
				if satellite.UUID != "" {
					currentGroup.Members = append(currentGroup.Members, satellite)
				}
			}
		}
	}

	return state
}

// parseDidlFavorites parses DIDL-Lite favorites result.
func parseDidlFavorites(payload []byte) []FavoriteItem {
	decoder := xml.NewDecoder(bytes.NewReader(payload))
	var favorites []FavoriteItem
	var current *FavoriteItem

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch se := tok.(type) {
		case xml.StartElement:
			switch se.Name.Local {
			case "item":
				fav := FavoriteItem{}
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "id":
						fav.ID = attr.Value
					case "parentID":
						fav.ParentID = attr.Value
					}
				}
				favorites = append(favorites, fav)
				current = &favorites[len(favorites)-1]
			case "title":
				if current != nil {
					var value string
					if err := decoder.DecodeElement(&value, &se); err == nil {
						current.Title = strings.TrimSpace(value)
					}
				}
			case "class":
				if current != nil {
					var value string
					if err := decoder.DecodeElement(&value, &se); err == nil {
						current.UpnpClass = strings.TrimSpace(value)
					}
				}
			case "albumArtURI":
				if current != nil {
					var value string
					if err := decoder.DecodeElement(&value, &se); err == nil {
						current.AlbumArtURI = strings.TrimSpace(value)
					}
				}
			case "res":
				if current != nil {
					var value string
					if err := decoder.DecodeElement(&value, &se); err == nil {
						current.Resource = strings.TrimSpace(value)
					}
					for _, attr := range se.Attr {
						if attr.Name.Local == "protocolInfo" {
							current.ProtocolInfo = attr.Value
						}
					}
				}
			case "desc":
				if current != nil {
					var value string
					if err := decoder.DecodeElement(&value, &se); err == nil {
						current.ResourceMetaData = strings.TrimSpace(value)
					}
				}
			case "ordinal":
				if current != nil {
					var value string
					if err := decoder.DecodeElement(&value, &se); err == nil {
						current.Ordinal = strings.TrimSpace(value)
					}
				}
			case "type":
				if current != nil {
					var value string
					if err := decoder.DecodeElement(&value, &se); err == nil {
						current.FavoriteType = strings.TrimSpace(value)
					}
				}
			case "description":
				if current != nil {
					var value string
					if err := decoder.DecodeElement(&value, &se); err == nil {
						current.ServiceName = strings.TrimSpace(value)
					}
				}
			case "resMD":
				if current != nil {
					var value string
					if err := decoder.DecodeElement(&value, &se); err == nil {
						// resMD contains the DIDL-Lite metadata for playback
						current.ResourceMetaData = strings.TrimSpace(value)
					}
				}
			}
		}
	}

	return favorites
}

// parseAlarmList parses ListAlarms response (minimal fields).
func parseAlarmList(payload []byte) AlarmListResult {
	result := AlarmListResult{}
	result.AlarmListVersion = parseTextValue(payload, "CurrentAlarmListVersion")

	alarmListXML := parseTextValue(payload, "CurrentAlarmList")
	if alarmListXML == "" {
		return result
	}

	decoder := xml.NewDecoder(strings.NewReader(alarmListXML))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch se := tok.(type) {
		case xml.StartElement:
			if se.Name.Local == "Alarm" {
				alarm := Alarm{}
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "ID":
						alarm.ID = attr.Value
					case "StartTime":
						alarm.StartTime = attr.Value
					case "Duration":
						alarm.Duration = attr.Value
					case "Recurrence":
						alarm.Recurrence = attr.Value
					case "Enabled":
						alarm.Enabled = attr.Value == "1" || strings.EqualFold(attr.Value, "true")
					case "RoomUUID":
						alarm.RoomUUID = attr.Value
					case "ProgramURI":
						alarm.ProgramURI = attr.Value
					case "ProgramMetaData":
						alarm.ProgramMetaData = attr.Value
					case "PlayMode":
						alarm.PlayMode = attr.Value
					case "Volume":
						alarm.Volume, _ = strconv.Atoi(attr.Value)
					case "IncludeLinkedZones":
						alarm.IncludeLinkedZones = attr.Value == "1" || strings.EqualFold(attr.Value, "true")
					}
				}
				result.Alarms = append(result.Alarms, alarm)
			}
		}
	}

	return result
}

// Helpers for predictable defaults in tests.
var now = func() time.Time { return time.Now() }
