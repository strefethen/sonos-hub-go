package discovery

import (
	"bufio"
	"context"
	"net"
	"strings"
	"time"
)

const (
	ssdpAddr   = "239.255.255.250:1900"
	ssdpTarget = "urn:schemas-upnp-org:device:ZonePlayer:1"
)

type Response struct {
	Location string
	USN      string
	Headers  map[string]string
	FromIP   string
}

// Discover performs SSDP M-SEARCH with multi-pass behavior.
func Discover(ctx context.Context, passes int, passInterval, timeout time.Duration) ([]Response, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	addr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil, err
	}

	responses := make(map[string]Response)

	for pass := 0; pass < passes; pass++ {
		if err := sendSearch(conn, addr); err != nil {
			return nil, err
		}
		if pass < passes-1 {
			select {
			case <-ctx.Done():
				return mapToSlice(responses), ctx.Err()
			case <-time.After(passInterval):
			}
		}
	}

	deadline := time.Now().Add(timeout)
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}

	buf := make([]byte, 2048)
	for {
		n, raddr, err := conn.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return mapToSlice(responses), err
		}

		resp := parseResponse(string(buf[:n]))
		if resp.Location == "" || resp.USN == "" {
			continue
		}
		resp.FromIP = raddr.String()

		// Deduplicate by USN
		if _, exists := responses[resp.USN]; !exists {
			responses[resp.USN] = resp
		}
	}

	return mapToSlice(responses), nil
}

func sendSearch(conn net.PacketConn, addr *net.UDPAddr) error {
	msg := strings.Join([]string{
		"M-SEARCH * HTTP/1.1",
		"HOST: " + ssdpAddr,
		"MAN: \"ssdp:discover\"",
		"MX: 2",
		"ST: " + ssdpTarget,
		"",
		"",
	}, "\r\n")

	_, err := conn.WriteTo([]byte(msg), addr)
	return err
}

func parseResponse(raw string) Response {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	headers := make(map[string]string)

	// Skip status line
	if scanner.Scan() {
		// no-op
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		headers[key] = value
	}

	return Response{
		Location: headers["LOCATION"],
		USN:      headers["USN"],
		Headers:  headers,
	}
}

func mapToSlice(responses map[string]Response) []Response {
	result := make([]Response, 0, len(responses))
	for _, r := range responses {
		result = append(result, r)
	}
	return result
}
