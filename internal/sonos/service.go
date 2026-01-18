package sonos

import (
	"context"
	"errors"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/devices"
	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

// Service exposes Sonos operations needed by routes.
type Service struct {
	DeviceService   *devices.Service
	SoapClient      *soap.Client
	DefaultDeviceIP string
	SoapTimeout     time.Duration
}

func NewService(deviceService *devices.Service, soapClient *soap.Client, defaultIP string, timeout time.Duration) *Service {
	return &Service{
		DeviceService:   deviceService,
		SoapClient:      soapClient,
		DefaultDeviceIP: defaultIP,
		SoapTimeout:     timeout,
	}
}

// ResolveDeviceIP resolves the IP for a device ID, falling back to default.
func (service *Service) ResolveDeviceIP(deviceID string) (string, error) {
	if service.DeviceService == nil {
		if service.DefaultDeviceIP == "" {
			return "", errors.New("no device resolver available")
		}
		return service.DefaultDeviceIP, nil
	}

	ip, err := service.DeviceService.ResolveDeviceIP(deviceID)
	if err != nil {
		return "", err
	}
	if ip == "" {
		if service.DefaultDeviceIP == "" {
			return "", errors.New("device not found")
		}
		return service.DefaultDeviceIP, nil
	}
	return ip, nil
}

func (service *Service) ListAlarms(deviceIP string) (soap.AlarmListResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.ListAlarms(ctx, deviceIP)
}

func (service *Service) BrowseFavorites(start, count int) (soap.BrowseResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.Browse(ctx, service.DefaultDeviceIP, "FV:2", "BrowseDirectChildren", "*", start, count)
}

func (service *Service) Stop(deviceIP string) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.Stop(ctx, deviceIP)
}

func (service *Service) Pause(deviceIP string) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.Pause(ctx, deviceIP)
}

func (service *Service) Play(deviceIP string) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.Play(ctx, deviceIP)
}

func (service *Service) Next(deviceIP string) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.Next(ctx, deviceIP)
}

func (service *Service) Previous(deviceIP string) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.Previous(ctx, deviceIP)
}

func (service *Service) GetTransportInfo(deviceIP string) (soap.TransportInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.GetTransportInfo(ctx, deviceIP)
}

func (service *Service) GetPositionInfo(deviceIP string) (soap.PositionInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.GetPositionInfo(ctx, deviceIP)
}

func (service *Service) GetMediaInfo(deviceIP string) (soap.MediaInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.GetMediaInfo(ctx, deviceIP)
}

func (service *Service) GetVolume(deviceIP string) (soap.VolumeInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.GetVolume(ctx, deviceIP)
}

func (service *Service) SetVolume(deviceIP string, level int) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.SetVolume(ctx, deviceIP, level)
}

func (service *Service) GetMute(deviceIP string) (soap.MuteInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.GetMute(ctx, deviceIP)
}

func (service *Service) GetZoneGroupState(deviceIP string) (soap.ZoneGroupState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.GetZoneGroupState(ctx, deviceIP)
}

func (service *Service) GetZoneAttributes(deviceIP string) (soap.ZoneAttributes, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.GetZoneAttributes(ctx, deviceIP)
}

func (service *Service) SetAVTransportURI(deviceIP string, uri string) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.SetAVTransportURI(ctx, deviceIP, uri, "")
}

func (service *Service) BecomeCoordinatorOfStandaloneGroup(deviceIP string) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.BecomeCoordinatorOfStandaloneGroup(ctx, deviceIP)
}
