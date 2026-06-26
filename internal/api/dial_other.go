//go:build !windows

package api

import (
	"context"
	"net"
)

// dialControl dials the mihomo control Unix socket.
func dialControl(ctx context.Context, socketPath string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", socketPath)
}
