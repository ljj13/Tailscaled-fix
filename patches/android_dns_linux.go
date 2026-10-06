//go:build linux && !android

package main

import (
	"context"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
	"tailscale.com/tsconst"
)

// DNS must bypass the tunnel and Android VPN just like control-plane sockets.
// Server selection, UDP/TCP retry and file reload remain Go resolver behavior.
func init() {
	net.DefaultResolver.PreferGo = true
	net.DefaultResolver.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{Control: func(_, _ string, c syscall.RawConn) error {
			var sockErr error
			err := c.Control(func(fd uintptr) {
				sockErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_MARK, tsconst.LinuxBypassMarkNum)
			})
			if err != nil {
				return err
			}
			return sockErr
		}}
		return d.DialContext(ctx, network, address)
	}
}
