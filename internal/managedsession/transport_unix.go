//go:build !windows

package managedsession

import (
	"context"
	"errors"
	"net"
	"strings"
)

// dialManagedEndpoint opens the owner-local Unix endpoint used by Harbor on Unix-like hosts.
func dialManagedEndpoint(ctx context.Context, endpoint string) (net.Conn, error) {
	if strings.HasPrefix(endpoint, `\\.\pipe\`) {
		return nil, errors.New("managed session named-pipe endpoints are not supported on this host")
	}
	return (&net.Dialer{}).DialContext(ctx, "unix", endpoint)
}
