// android-dns discovers Android LinkProperties DNS and publishes a private
// bootstrap resolver file. It never changes Android DNS, routes, or tailnet state.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const bypassMark = 0x10020000

func validServers(input []string, iface string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range input {
		s = strings.TrimSpace(strings.TrimPrefix(s, "/"))
		ip, err := netip.ParseAddr(s)
		if err != nil {
			continue
		}
		ip = ip.Unmap()
		if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.String() == "100.100.100.100" || ip.String() == "fd7a:115c:a1e0::53" {
			continue
		}
		if ip.IsLinkLocalUnicast() && ip.Is6() && ip.Zone() == "" {
			if iface == "" {
				continue
			}
			ip = ip.WithZone(iface)
		}
		s = ip.String()
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if len(out) == 12 {
			break
		}
	}
	return out
}

func command(ctx context.Context, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func markedControl(mark bool) func(string, string, syscall.RawConn) error {
	return func(_, _ string, c syscall.RawConn) error {
		var sockErr error
		err := c.Control(func(fd uintptr) {
			if mark {
				sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, bypassMark)
			}
		})
		if err != nil {
			return err
		}
		return sockErr
	}
}

func probeDNS(ctx context.Context, address, domain string, mark bool) error {
	ctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		d := net.Dialer{Control: markedControl(mark)}
		return d.DialContext(ctx, network, address)
	}}
	ips, err := r.LookupIP(ctx, "ip4", domain)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			copy := *dnsErr
			copy.Server = address
			return &copy
		}
		return err
	}
	if len(ips) == 0 {
		return errors.New("no A answers")
	}
	return nil
}

func working(servers []string, probe func(string) error) ([]string, []string) {
	results := make([]error, len(servers))
	var wg sync.WaitGroup
	for i, s := range servers {
		wg.Add(1)
		go func(i int, s string) { defer wg.Done(); results[i] = probe(s) }(i, s)
	}
	wg.Wait()
	var ok, detail []string
	for i, s := range servers {
		if results[i] == nil {
			ok = append(ok, s)
			detail = append(detail, s+":ok")
		} else {
			detail = append(detail, s+":"+strings.Join(strings.Fields(results[i].Error()), " "))
		}
	}
	return ok, detail
}

func choose(android []string, source string, fallback []string, probe func(string) error) ([]string, string, bool, []string) {
	servers, source, reachable, _, details := chooseCached(android, source, fallback, verifiedDNS{}, false, probe)
	return servers, source, reachable, details
}

func chooseCached(android []string, source string, fallback []string, old verifiedDNS, retainable bool, probe func(string) error) ([]string, string, bool, bool, []string) {
	servers, details := working(android, probe)
	if len(servers) > 0 {
		return servers, source, true, false, details
	}
	if retainable {
		ok, more := working(old.Servers, probe)
		details = append(details, more...)
		if len(ok) > 0 {
			return ok, old.Source, true, false, details
		}
	}
	ok, more := working(fallback, probe)
	details = append(details, more...)
	if len(ok) > 0 {
		return ok, "public-fallback", true, false, details
	}
	if retainable {
		return old.Servers, old.Source, false, true, details
	}
	if len(android) > 0 {
		return android, source + "-unverified", false, false, details
	}
	return fallback, "public-fallback-unverified", false, false, details
}

func writeAtomic(path string, data []byte) error {
	old, err := os.ReadFile(path)
	if err == nil && string(old) == string(data) {
		return nil
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".dns-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func selectionStatus(s Selection, hint string) string {
	return fmt.Sprintf("dns_network=%s\ndns_transport=%s\ndns_active_vpn=%s\ndns_underlying=%s\ndns_iface=%s\ndns_excluded=%s\ndns_selection_reason=%s\ndns_route_hint=%s\ndns_physical_route=%s\n", s.ID, s.Transport, s.VPN, s.Underlying, s.Iface, s.Excluded, s.Reason, hint, s.Route)
}

func routeSnapshot(server string) string {
	host := strings.Split(server, "%")[0]
	family := "-4"
	if strings.Contains(host, ":") {
		family = "-6"
	}
	return strings.Join(strings.Fields(command(context.Background(), "ip", family, "route", "get", host, "mark", "0x10020000")), " ")
}

func publishedServers(dir string) []string {
	data, _ := os.ReadFile(filepath.Join(dir, "bootstrap-resolv.conf"))
	var servers []string
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "nameserver" {
			servers = append(servers, f[1])
		}
	}
	return servers
}

func run() error {
	dir := flag.String("dir", "/data/adb/tailscale", "state directory")
	iface := flag.String("iface", "", "ordinary route hint (not authoritative)")
	domain := flag.String("domain", "controlplane.tailscale.com", "DNS reachability test name")
	fallback := flag.String("fallback", "1.1.1.1,8.8.8.8,9.9.9.9,223.5.5.5,119.29.29.29", "fallback servers; empty disables public fallback")
	caller := flag.String("caller", "manual", "diagnostic caller label")
	check := flag.Bool("check", false, "check published servers without changing them")
	networkOnly := flag.Bool("network", false, "read-only physical network and route discovery")
	flag.Parse()
	discoveryStart := time.Now()
	discoveryCtx, discoveryCancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer discoveryCancel()
	if *networkOnly {
		s := selectNetwork(command(discoveryCtx, "dumpsys", "connectivity"), *iface)
		fmt.Print(selectionStatus(s, *iface))
		if s.ID == "" {
			return errors.New("no physical Android network")
		}
		return nil
	}
	if err := os.MkdirAll(*dir, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(*dir, ".dns.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("DNS refresh already running: %w", err)
	}
	s, android, source := discover(discoveryCtx, *iface)
	discoveryMS := time.Since(discoveryStart).Milliseconds()
	// Start a fresh budget AFTER discovery. Manual and watchdog use the same
	// marked sockets, no SO_BINDTODEVICE, and the same probe deadline.
	probe := func(server string) error {
		ctx, cancel := freshProbeContext()
		defer cancel()
		return probeDNS(ctx, net.JoinHostPort(server, "53"), *domain, true)
	}
	if *check {
		fmt.Print(selectionStatus(s, *iface))
		servers := publishedServers(*dir)
		before := ""
		if len(servers) > 0 {
			before = routeSnapshot(servers[0])
		}
		started := time.Now()
		ok, details := working(servers, probe)
		fmt.Printf("dns_probe_caller=%s\ndns_probe_mark=0x10020000\ndns_discovery_ms=%d\ndns_probe_ms=%d\ndns_route_before=%s\n", *caller, discoveryMS, time.Since(started).Milliseconds(), before)
		if len(servers) > 0 {
			fmt.Printf("dns_route_after=%s\n", routeSnapshot(servers[0]))
		}
		fmt.Println(strings.Join(details, "\n"))
		if len(ok) == 0 {
			return errors.New("no reachable bootstrap DNS")
		}
		return nil
	}
	cachePath := filepath.Join(*dir, "dns-verified.json")
	old := readVerified(cachePath)
	old = allowedCache(old, validServers(strings.Split(*fallback, ","), s.Iface))
	retainable := canRetain(old, s, time.Now())
	before := ""
	candidates := append(append([]string{}, android...), validServers(strings.Split(*fallback, ","), s.Iface)...)
	probeTarget := ""
	if len(candidates) > 0 {
		probeTarget = candidates[0]
		for _, candidate := range candidates {
			if !strings.Contains(candidate, ":") {
				probeTarget = candidate
				break
			}
		}
		before = routeSnapshot(probeTarget)
	}
	started := time.Now()
	servers, source, reachable, retained, details := chooseCached(android, source, validServers(strings.Split(*fallback, ","), s.Iface), old, retainable, probe)
	probeMS := time.Since(started).Milliseconds()
	if len(servers) == 0 {
		if len(validServers(strings.Split(*fallback, ","), s.Iface)) == 0 {
			if err := revokePublicBootstrap(*dir); err != nil {
				return err
			}
		}
		return errors.New("no DNS candidates; public fallback is disabled")
	}
	if len(servers) > 3 {
		servers = servers[:3]
	}
	// A network switch during probes must not publish old-network answers as
	// verified on the new network. Leave the existing resolver for this tick;
	// the watchdog repeats discovery and probing on the next tick.
	afterSelection := selectNetwork(command(context.Background(), "dumpsys", "connectivity"), *iface)
	changed := s.ID != afterSelection.ID || s.Iface != afterSelection.Iface || s.Underlying != afterSelection.Underlying
	if changed {
		status := selectionStatus(afterSelection, *iface) + fmt.Sprintf("dns_source=network-changed-during-probe\ndns_servers=%s\ndns_reachable=false\ndns_retained=true\ndns_checked=%s\n", strings.Join(publishedServers(*dir), ","), time.Now().Format(time.RFC3339))
		_ = writeAtomic(filepath.Join(*dir, "dns-status"), []byte(status))
		fmt.Print(status)
		if len(validServers(publishedServers(*dir), s.Iface)) == 0 {
			return errors.New("network changed before first bootstrap; watchdog will retry startup")
		}
		return nil
	}
	contents := "# Android bootstrap DNS; managed by android-dns\n"
	for _, server := range servers {
		contents += "nameserver " + server + "\n"
	}
	contents += "options timeout:2 attempts:2\n"
	// A failed refresh preserving the same verified list must not touch even
	// comments/mtime of the existing bootstrap file.
	if !retained || strings.Join(publishedServers(*dir), ",") != strings.Join(servers, ",") {
		if err := writeAtomic(filepath.Join(*dir, "bootstrap-resolv.conf"), []byte(contents)); err != nil {
			return err
		}
	}
	if reachable && s.ID != "" {
		old = verifiedDNS{Network: s.ID, Iface: s.Iface, Source: source, Servers: servers, Verified: time.Now(), BootID: bootID()}
		data, err := json.Marshal(old)
		if err != nil {
			return err
		}
		if err = writeAtomic(cachePath, append(data, '\n')); err != nil {
			return err
		}
	} else if !retainable {
		_ = os.Remove(cachePath)
	}
	after := ""
	if probeTarget != "" {
		after = routeSnapshot(probeTarget)
	}
	status := selectionStatus(s, *iface) + fmt.Sprintf("dns_source=%s\ndns_servers=%s\ndns_reachable=%t\ndns_retained=%t\ndns_last_verified=%s\ndns_checked=%s\ndns_probe=%s\ndns_probe_caller=%s\ndns_probe_mark=0x10020000\ndns_discovery_ms=%d\ndns_probe_ms=%d\ndns_route_before=%s\ndns_route_after=%s\ndns_details=%s\n", source, strings.Join(servers, ","), reachable, retained, old.Verified.Format(time.RFC3339), time.Now().Format(time.RFC3339), *domain, *caller, discoveryMS, probeMS, before, after, strings.Join(details, "; "))
	if err := writeAtomic(filepath.Join(*dir, "dns-status"), []byte(status)); err != nil {
		return err
	}
	fmt.Print(status)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "android-dns:", err)
		os.Exit(1)
	}
}
