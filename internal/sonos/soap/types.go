package soap

// Service identifies a Sonos UPnP service.
type Service string

const (
	ServiceAVTransport       Service = "AVTransport"
	ServiceRenderingControl  Service = "RenderingControl"
	ServiceContentDirectory  Service = "ContentDirectory"
	ServiceZoneGroupTopology Service = "ZoneGroupTopology"
	ServiceDeviceProperties  Service = "DeviceProperties"
	ServiceAlarmClock        Service = "AlarmClock"
)

var serviceTypes = map[Service]string{
	ServiceAVTransport:       "urn:schemas-upnp-org:service:AVTransport:1",
	ServiceRenderingControl:  "urn:schemas-upnp-org:service:RenderingControl:1",
	ServiceContentDirectory:  "urn:schemas-upnp-org:service:ContentDirectory:1",
	ServiceZoneGroupTopology: "urn:upnp-org:serviceId:ZoneGroupTopology",
	ServiceDeviceProperties:  "urn:upnp-org:serviceId:DeviceProperties",
	ServiceAlarmClock:        "urn:schemas-upnp-org:service:AlarmClock:1",
}

var controlPaths = map[Service]string{
	ServiceAVTransport:       "/MediaRenderer/AVTransport/Control",
	ServiceRenderingControl:  "/MediaRenderer/RenderingControl/Control",
	ServiceContentDirectory:  "/MediaServer/ContentDirectory/Control",
	ServiceZoneGroupTopology: "/ZoneGroupTopology/Control",
	ServiceDeviceProperties:  "/DeviceProperties/Control",
	ServiceAlarmClock:        "/AlarmClock/Control",
}
