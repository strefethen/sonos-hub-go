package soap

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Client handles SOAP requests to Sonos devices.
type Client struct {
	httpClient *http.Client
	timeout    time.Duration
}

// NewClient creates a SOAP client with the given timeout.
// Uses connection pooling for better performance when making multiple requests.
func NewClient(timeout time.Duration) *Client {
	return &Client{
		timeout: timeout,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				DialContext:         (&net.Dialer{Timeout: timeout}).DialContext,
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// ExecuteAction sends a SOAP request and returns the raw response body.
func (c *Client) ExecuteAction(
	ctx context.Context,
	ip string,
	service Service,
	action string,
	args map[string]string,
) ([]byte, error) {
	serviceType := serviceTypes[service]
	controlPath := controlPaths[service]
	if serviceType == "" || controlPath == "" {
		return nil, fmt.Errorf("unknown service: %s", service)
	}

	body := buildEnvelope(serviceType, action, args)
	url := fmt.Sprintf("http://%s:1400%s", ip, controlPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "text/xml; charset=\"utf-8\"")
	req.Header.Set("SOAPACTION", fmt.Sprintf("\"%s#%s\"", serviceType, action))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, &SonosTimeoutError{Action: action}
		}
		return nil, &SonosUnreachableError{Action: action, Err: err}
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		code, desc := parseSoapFault(payload)
		if code != "" {
			return nil, &SonosRejectedError{Action: action, Code: code, Description: desc}
		}
		return nil, fmt.Errorf("sonos action %s failed: http %d", action, resp.StatusCode)
	}

	return payload, nil
}

func buildEnvelope(serviceType, action string, args map[string]string) []byte {
	var buf strings.Builder
	buf.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>")
	buf.WriteString("<s:Envelope xmlns:s=\"http://schemas.xmlsoap.org/soap/envelope/\" s:encodingStyle=\"http://schemas.xmlsoap.org/soap/encoding/\">")
	buf.WriteString("<s:Body>")
	buf.WriteString("<u:")
	buf.WriteString(action)
	buf.WriteString(" xmlns:u=\"")
	buf.WriteString(serviceType)
	buf.WriteString("\">")

	for key, value := range args {
		buf.WriteString("<")
		buf.WriteString(key)
		buf.WriteString(">")
		buf.WriteString(escapeXML(value))
		buf.WriteString("</")
		buf.WriteString(key)
		buf.WriteString(">")
	}

	buf.WriteString("</u:")
	buf.WriteString(action)
	buf.WriteString(">")
	buf.WriteString("</s:Body>")
	buf.WriteString("</s:Envelope>")

	return []byte(buf.String())
}

func escapeXML(input string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(input)); err != nil {
		return input
	}
	return b.String()
}

func parseSoapFault(payload []byte) (string, string) {
	decoder := xml.NewDecoder(bytes.NewReader(payload))
	var code string
	var desc string

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch se := tok.(type) {
		case xml.StartElement:
			switch se.Name.Local {
			case "errorCode":
				var value string
				if err := decoder.DecodeElement(&value, &se); err == nil {
					code = strings.TrimSpace(value)
				}
			case "errorDescription":
				var value string
				if err := decoder.DecodeElement(&value, &se); err == nil {
					desc = strings.TrimSpace(value)
				}
			}
		}
	}

	return code, desc
}
