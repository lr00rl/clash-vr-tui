package probe

import (
	"fmt"
	"net"
	"runtime"
	"strconv"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

// Mode selects how node latency is measured.
type Mode int

const (
	// ModeHTTP uses mihomo's HTTP delay test (latency through the tunnel).
	ModeHTTP Mode = iota
	// ModeTCP measures a raw TCP connect to the node's server:port.
	ModeTCP
	// ModeICMP measures an ICMP echo round-trip to the node's server.
	ModeICMP
)

func (m Mode) String() string {
	switch m {
	case ModeTCP:
		return "TCP"
	case ModeICMP:
		return "ICMP"
	default:
		return "HTTP"
	}
}

// Next cycles to the next test mode.
func (m Mode) Next() Mode {
	return (m + 1) % 3
}

// NeedsEndpoints reports whether the mode requires server addresses from config.
func (m Mode) NeedsEndpoints() bool {
	return m == ModeTCP || m == ModeICMP
}

// TCPPing measures the time to establish a TCP connection to server:port.
// Returns latency in milliseconds, or an error (treated as a timeout).
func TCPPing(server string, port int, timeout time.Duration) (int, error) {
	if server == "" || port == 0 {
		return 0, fmt.Errorf("unknown server endpoint")
	}
	addr := net.JoinHostPort(server, strconv.Itoa(port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return 0, err
	}
	_ = conn.Close()
	ms := int(time.Since(start).Milliseconds())
	if ms < 1 {
		ms = 1
	}
	return ms, nil
}

// ICMPPing measures an ICMP echo round-trip to host (an IP or hostname).
// Uses unprivileged (UDP) ping where the OS allows it; on Linux this needs
// net.ipv4.ping_group_range or root.
func ICMPPing(host string, timeout time.Duration) (int, error) {
	if host == "" {
		return 0, fmt.Errorf("unknown server endpoint")
	}
	pinger, err := probing.NewPinger(host)
	if err != nil {
		return 0, err
	}
	pinger.Count = 1
	pinger.Timeout = timeout
	// Unprivileged datagram ping works on macOS and on Linux when
	// net.ipv4.ping_group_range is set; fall back to privileged on Linux.
	pinger.SetPrivileged(runtime.GOOS == "linux")
	if err := pinger.Run(); err != nil {
		// Retry unprivileged on Linux in case raw sockets are unavailable.
		if runtime.GOOS == "linux" {
			pinger.SetPrivileged(false)
			if err2 := pinger.Run(); err2 != nil {
				return 0, err
			}
		} else {
			return 0, err
		}
	}
	stats := pinger.Statistics()
	if stats.PacketsRecv == 0 {
		return 0, fmt.Errorf("timeout")
	}
	ms := int(stats.AvgRtt.Milliseconds())
	if ms < 1 {
		ms = 1
	}
	return ms, nil
}
