// android-dns discovers Android LinkProperties DNS and publishes a private
// bootstrap resolver file. It never changes Android DNS, routes, or tailnet state.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

const bypassMark = 0x10020000

var dnsList = regexp.MustCompile(`DnsAddresses:\s*\[([^\]]*)\]`)
var ifaceName = regexp.MustCompile(`InterfaceName:\s*([^\s,}]+)`)
var networkID = regexp.MustCompile(`network\{([0-9]+)\}`)
var activeID = regexp.MustCompile(`Active default network:\s*([0-9]+)`)
var vpnNetwork = regexp.MustCompile(`ni\{VPN\b|\btype:\s*VPN\b|Transports:\s*VPN\b`)

func connectivityDNS(dump, iface string) []string {
	active := activeID.FindStringSubmatch(dump)
	// Restrict parsing to current NetworkAgentInfo records, never request logs.
	if i := strings.Index(dump, "Current Networks:"); i >= 0 {
		dump = dump[i:]
	}
	if i := strings.Index(dump, "Network Requests:"); i >= 0 {
		dump = dump[:i]
	}
	// Active default network usually precedes Current Networks; interface matching
	// is preferred because a VPN may itself be Android's default network.
	starts := regexp.MustCompile(`(?m)^\s*(?:NetworkAgentInfo|networkAgentInfo)\s*[\[{]`).FindAllStringIndex(dump, -1)
	for i, pos := range starts {
		end := len(dump)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		block := dump[pos[0]:end]
		name := ifaceName.FindStringSubmatch(block)
		id := networkID.FindStringSubmatch(block)
		if len(name) < 2 {
			continue
		}
		// CLAT interfaces are stacked links; DNS belongs to their parent.
		match := false
		for _, n := range ifaceName.FindAllStringSubmatch(block, -1) {
			if iface != "" && n[1] == iface {
				match = true
			}
		}
		if vpnNetwork.MatchString(block) {
			continue
		}
		if iface == "" && len(active) == 2 && len(id) == 2 && id[1] == active[1] {
			match = true
		}
		if !match {
			continue
		}
		dns := dnsList.FindStringSubmatch(block)
		if len(dns) == 2 {
			return validServers(strings.FieldsFunc(dns[1], func(r rune) bool { return r == ',' || r == '/' || r == ' ' }), name[1])
		}
	}
	return nil
}

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

func discover(ctx context.Context, iface string) ([]string, string) {
	dump := command(ctx, "dumpsys", "connectivity")
	if dns := connectivityDNS(dump, iface); len(dns) > 0 {
		return dns, "android-linkproperties"
	}
	// Older Android releases expose DNS via properties; newer releases don't.
	var props []string
	if iface != "" {
		for _, n := range []string{"1", "2", "3", "4"} {
			props = append(props, command(ctx, "getprop", "net."+iface+".dns"+n))
		}
	}
	if dns := validServers(props, iface); len(dns) > 0 {
		return dns, "android-interface-property"
	}
	props = nil
	for _, n := range []string{"1", "2", "3", "4"} {
		props = append(props, command(ctx, "getprop", "net.dns"+n))
	}
	return validServers(props, iface), "android-legacy-property"
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
	ok, details := working(android, probe)
	if len(ok) > 0 {
		return ok, source, true, details
	}
	ok, more := working(fallback, probe)
	details = append(details, more...)
	if len(ok) > 0 {
		return ok, "public-fallback", true, details
	}
	// Offline boot must still publish non-loopback candidates so reconnect works.
	if len(android) > 0 {
		return android, source + "-unverified", false, details
	}
	return fallback, "public-fallback-unverified", false, details
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

func run() error {
	dir := flag.String("dir", "/data/adb/tailscale", "state directory")
	iface := flag.String("iface", "", "active physical interface")
	domain := flag.String("domain", "controlplane.tailscale.com", "DNS reachability test name")
	fallback := flag.String("fallback", "1.1.1.1,8.8.8.8,9.9.9.9,223.5.5.5,119.29.29.29", "fallback servers; empty disables public fallback")
	check := flag.Bool("check", false, "check currently published servers without changing them")
	flag.Parse()
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
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	probe := func(s string) error { return probeDNS(ctx, net.JoinHostPort(s, "53"), *domain, true) }
	if *check {
		data, err := os.ReadFile(filepath.Join(*dir, "bootstrap-resolv.conf"))
		if err != nil {
			return err
		}
		var servers []string
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) == 2 && f[0] == "nameserver" {
				servers = append(servers, f[1])
			}
		}
		ok, details := working(servers, probe)
		fmt.Println(strings.Join(details, "\n"))
		if len(ok) == 0 {
			return errors.New("no reachable bootstrap DNS")
		}
		return nil
	}
	android, source := discover(ctx, *iface)
	servers, source, reachable, details := choose(android, source, validServers(strings.Split(*fallback, ","), *iface), probe)
	if len(servers) == 0 {
		return errors.New("no DNS candidates; public fallback is disabled")
	}
	if len(servers) > 3 {
		servers = servers[:3]
	}
	contents := "# Android bootstrap DNS; managed by android-dns\n"
	for _, s := range servers {
		contents += "nameserver " + s + "\n"
	}
	contents += "options timeout:2 attempts:2\n"
	if err := writeAtomic(filepath.Join(*dir, "bootstrap-resolv.conf"), []byte(contents)); err != nil {
		return err
	}
	status := fmt.Sprintf("dns_source=%s\ndns_servers=%s\ndns_iface=%s\ndns_reachable=%t\ndns_checked=%s\ndns_probe=%s\ndns_details=%s\n", source, strings.Join(servers, ","), *iface, reachable, time.Now().Format(time.RFC3339), *domain, strings.Join(details, "; "))
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
