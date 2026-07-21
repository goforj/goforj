//go:build windows

package managedsession

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
)

// dialManagedEndpoint opens Harbor's owner-authenticated named pipe on Windows.
func dialManagedEndpoint(ctx context.Context, endpoint string) (net.Conn, error) {
	if !strings.HasPrefix(endpoint, `\\.\pipe\`) {
		return nil, errors.New("managed session endpoint is not a Windows named pipe")
	}
	return winio.DialPipeContext(ctx, endpoint)
}
