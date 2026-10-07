package main

// Optional passive NETLINK_ROUTE observer. Only signals its own watchdog parent;
// the existing watchdog remains the sole owner of reconciliation and Re-STUN.
import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type eventGate struct {
	first, last, check time.Time
	dirty              bool
}

func (g *eventGate) event(now time.Time) {
	if !g.dirty {
		g.first = now
	}
	g.dirty, g.last = true, now
}
func (g *eventGate) ready(now time.Time) bool {
	return g.dirty && now.Sub(g.check) >= 2*time.Second && (now.Sub(g.last) >= 750*time.Millisecond || now.Sub(g.first) >= 2*time.Second)
}
func (g *eventGate) checked(now time.Time, valid bool) {
	g.check = now
	// No endless discovery polling in airplane mode or when Android is absent.
	// A later kernel event starts another bounded attempt; watchdog still polls.
	if valid || now.Sub(g.first) >= 10*time.Second {
		g.dirty = false
	}
}
func validWatchParent(requested, actual int) bool { return requested > 1 && requested == actual }

func physicalFingerprint(network string, addresses []string, routes string) (string, bool) {
	values := map[string]string{}
	for _, line := range strings.Split(network, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok {
			values[k] = strings.TrimSpace(v)
		}
	}
	iface := values["dns_iface"]
	id, err := strconv.ParseUint(values["dns_network"], 10, 32)
	if err != nil || id == 0 || !physical(iface) {
		return "", false
	}
	items := []string{fmt.Sprintf("network=%d interface=%s", id, iface)}
	for _, value := range addresses {
		ip, err := netip.ParseAddr(value)
		if err == nil && ip.IsGlobalUnicast() && !ip.IsLinkLocalUnicast() {
			items = append(items, "address="+ip.String())
		}
	}
	for _, line := range strings.Split(routes, "\n") {
		fields := strings.Fields(line)
		selected := false
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "dev" && fields[i+1] == iface {
				selected = true
			}
		}
		if !selected {
			continue
		}
		stable := []string{}
		for i := 0; i < len(fields); i++ {
			if fields[i] == "expires" {
				i++
				continue
			}
			stable = append(stable, fields[i])
		}
		items = append(items, "ipv6-route="+strings.Join(stable, " "))
	}
	sort.Strings(items)
	return strings.Join(items, "\n"), true
}

func watchFingerprint(ctx context.Context, dir string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	network := runCommand(ctx, filepath.Join(dir, "bin", "android-dns"), "--network")
	if network.Error != "" || network.Truncated {
		return "", false
	}
	var iface string
	for _, line := range strings.Split(network.Stdout, "\n") {
		if strings.HasPrefix(line, "dns_iface=") {
			iface = strings.TrimSpace(strings.TrimPrefix(line, "dns_iface="))
		}
	}
	i, err := net.InterfaceByName(iface)
	if err != nil {
		return "", false
	}
	all, err := i.Addrs()
	if err != nil {
		return "", false
	}
	addresses := []string{}
	for _, a := range all {
		if p, err := netip.ParsePrefix(a.String()); err == nil {
			addresses = append(addresses, p.Addr().String())
		}
	}
	routes := runCommand(ctx, "ip", "-6", "route", "show", "table", "all")
	if routes.Error != "" || routes.Truncated {
		return "", false
	}
	return physicalFingerprint(network.Stdout, addresses, routes.Stdout)
}

func watchNetwork(parent int, dir string) error {
	if !validWatchParent(parent, os.Getppid()) {
		return errors.New("watch parent must be this process's actual watchdog parent")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW|syscall.SOCK_CLOEXEC, syscall.NETLINK_ROUTE)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	// Linux RTMGRP_LINK, IPV4_IFADDR/ROUTE, IPV6_IFADDR/ROUTE. No mark layout assumptions.
	if err = syscall.Bind(fd, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK, Groups: 1 | 0x10 | 0x40 | 0x100 | 0x400}); err != nil {
		return err
	}
	if err = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &syscall.Timeval{Usec: 250000}); err != nil {
		return err
	}
	previous, valid := watchFingerprint(ctx, dir)
	g := eventGate{}
	if !valid {
		g.event(time.Now())
	}
	fmt.Println("network-observer: passive netlink ready (15s watchdog fallback retained)")
	buffer := make([]byte, 64*1024)
	for ctx.Err() == nil && validWatchParent(parent, os.Getppid()) {
		n, _, readErr := syscall.Recvfrom(fd, buffer, 0)
		if readErr != nil && readErr != syscall.EAGAIN && readErr != syscall.EWOULDBLOCK && readErr != syscall.EINTR && readErr != syscall.ENOBUFS {
			return readErr
		}
		now := time.Now()
		if readErr == syscall.ENOBUFS {
			g.event(now)
		}
		if n > 0 {
			messages, parseErr := syscall.ParseNetlinkMessage(buffer[:n])
			if parseErr != nil {
				g.event(now)
			}
			for _, m := range messages {
				switch m.Header.Type {
				case syscall.RTM_NEWLINK, syscall.RTM_DELLINK, syscall.RTM_NEWADDR, syscall.RTM_DELADDR, syscall.RTM_NEWROUTE, syscall.RTM_DELROUTE:
					g.event(now)
				}
			}
		}
		if !g.ready(now) {
			continue
		}
		current, ok := watchFingerprint(ctx, dir)
		g.checked(time.Now(), ok)
		if !ok {
			continue
		} // transient loss never publishes an empty physical selection
		if current != previous {
			if !validWatchParent(parent, os.Getppid()) {
				return nil
			}
			if err = syscall.Kill(parent, syscall.SIGUSR1); err != nil {
				return err
			}
			fmt.Println("network-observer: verified physical change; watchdog notified")
			previous = current
		}
	}
	return nil
}
