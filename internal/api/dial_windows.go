//go:build windows

package api

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
)

// dialControl dials the mihomo control named pipe on Windows.
func dialControl(ctx context.Context, pipePath string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, pipePath)
}
