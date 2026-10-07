// Read-only, bounded Tailscale diagnostics. Never writes resolver, prefs or routes.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const outputLimit = 128 * 1024

type CommandResult struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Error     string `json:"error,omitempty"`
	Timeout   bool   `json:"timeout"`
	Truncated bool   `json:"truncated"`
}
type Region struct {
	ID   int    `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}
type Node struct {
	HostName      string   `json:"hostname"`
	OS            string   `json:"os"`
	IPv4          []string `json:"ipv4"`
	IPv6          []string `json:"ipv6"`
	Online        *bool    `json:"online"`
	Active        *bool    `json:"active"`
	Relay         Region   `json:"relay"`
	CurAddr       string   `json:"current_endpoint"`
	PeerRelay     string   `json:"peer_relay"`
	Path          string   `json:"path"`
	LastWrite     string   `json:"last_write"`
	LastHandshake string   `json:"last_handshake"`
}
type Interface struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
}
type Endpoint struct {
	Address         string `json:"address"`
	IP              string `json:"ip"`
	Port            uint16 `json:"port"`
	Family          string `json:"family"`
	Scope           string `json:"scope"`
	Interface       string `json:"interface"`
	InterfaceSource string `json:"interface_source"`
}
type Netcheck struct {
	UDP                   *bool            `json:"udp"`
	IPv4                  *bool            `json:"ipv4"`
	IPv6                  *bool            `json:"ipv6"`
	MappingVariesByDestIP *bool            `json:"mapping_varies_by_dest_ip"`
	PortMapping           map[string]*bool `json:"port_mapping"`
	NearestDERP           Region           `json:"nearest_derp"`
	GlobalV4              string           `json:"global_ipv4"`
	GlobalV6              string           `json:"global_ipv6"`
	CaptivePortal         *bool            `json:"captive_portal"`
}
type Report struct {
	Schema       int                      `json:"schema"`
	Checked      string                   `json:"checked"`
	Backend      string                   `json:"backend"`
	Version      string                   `json:"version"`
	AcceptRoutes *bool                    `json:"accept_routes"`
	Self         Node                     `json:"self"`
	Peers        []Node                   `json:"peers"`
	Endpoints    []Endpoint               `json:"endpoints"`
	Interfaces   []Interface              `json:"interfaces"`
	UDPListeners []Endpoint               `json:"udp_listeners"`
	Netcheck     Netcheck                 `json:"netcheck"`
	Network      map[string]string        `json:"network"`
	OuterMark    string                   `json:"outer_mark"`
	Raw          map[string]CommandResult `json:"raw"`
	Errors       []string                 `json:"errors"`
	Notes        []string                 `json:"notes"`
}
type Options struct {
	CLI, Dir, PID, Mark, Peer string
	Args                      []string
	Export                    bool
	ExitAudit                 bool
	Ping                      bool
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	truncated bool
}

func (b *limitedBuffer) String() string { return b.buffer.String() }
func (b *limitedBuffer) Len() int       { return b.buffer.Len() }
func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := outputLimit - b.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	b.buffer.Write(p)
	return n, nil
}
func runCommand(ctx context.Context, name string, args ...string) CommandResult {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	cmd.WaitDelay = 200 * time.Millisecond
	var out, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err := cmd.Run()
	r := CommandResult{Stdout: out.String(), Stderr: stderr.String(), Timeout: ctx.Err() != nil, Truncated: out.truncated || stderr.truncated}
	if err != nil {
		r.Error = err.Error()
	}
	return r
}
func physical(name string) bool {
	return name != "" && name != "lo" && !strings.HasPrefix(name, "tun") && !strings.HasPrefix(name, "tailscale") && !strings.HasPrefix(name, "veth")
}
func interfaces() []Interface {
	out := []Interface{}
	all, _ := net.Interfaces()
	for _, item := range all {
		addresses, _ := item.Addrs()
		i := Interface{Name: item.Name, Addresses: []string{}}
		for _, a := range addresses {
			if p, err := netip.ParsePrefix(a.String()); err == nil {
				i.Addresses = append(i.Addresses, p.Addr().String())
			}
		}
		out = append(out, i)
	}
	return out
}
func endpoint(value string, ifaces []Interface, fallback string) Endpoint {
	e := Endpoint{Address: value, Family: "Unknown", Scope: "Unknown"}
	a, err := netip.ParseAddrPort(value)
	if err != nil {
		return e
	}
	ip := a.Addr().Unmap()
	e.IP, e.Port = ip.String(), a.Port()
	if ip.Is4() {
		e.Family = "IPv4"
	} else {
		e.Family = "IPv6"
	}
	switch {
	case ip.IsLoopback():
		e.Scope = "Loopback"
	case ip.IsLinkLocalUnicast():
		e.Scope = "Link-local"
	case ip.IsPrivate():
		e.Scope = "Private"
	case netip.MustParsePrefix("100.64.0.0/10").Contains(ip):
		e.Scope = "CGNAT"
	case ip.IsGlobalUnicast():
		e.Scope = "Public"
	default:
		e.Scope = "Other"
	}
	if physical(ip.Zone()) {
		e.Interface, e.InterfaceSource = ip.Zone(), "IPv6 zone"
		return e
	}
	for _, iface := range ifaces {
		if !physical(iface.Name) {
			continue
		}
		for _, candidate := range iface.Addresses {
			if local, err := netip.ParseAddr(candidate); err == nil && local.Unmap() == ip.WithZone("") {
				e.Interface, e.InterfaceSource = iface.Name, "exact local address"
				return e
			}
		}
	}
	if physical(fallback) && e.Scope == "Public" {
		e.Interface, e.InterfaceSource = fallback, "inferred physical egress; NAT mapping not proven"
	}
	return e
}

type statusNode struct {
	HostName, OS, Relay, CurAddr, PeerRelay, LastWrite, LastHandshake string
	TailscaleIPs, Addrs                                               []string
	Online, Active                                                    *bool
}

func node(p statusNode) Node {
	n := Node{HostName: p.HostName, OS: p.OS, Online: p.Online, Active: p.Active, Relay: Region{Code: p.Relay}, CurAddr: p.CurAddr, PeerRelay: p.PeerRelay, Path: "unknown", LastWrite: p.LastWrite, LastHandshake: p.LastHandshake, IPv4: []string{}, IPv6: []string{}}
	for _, text := range p.TailscaleIPs {
		if ip, err := netip.ParseAddr(text); err == nil {
			if ip.Is4() {
				n.IPv4 = append(n.IPv4, text)
			} else {
				n.IPv6 = append(n.IPv6, text)
			}
		}
	}
	switch {
	case p.Active != nil && *p.Active:
		switch {
		case p.PeerRelay != "":
			n.Path = "peer-relay"
		case p.CurAddr != "":
			n.Path = "direct"
		case p.Relay != "":
			n.Path = "derp"
		}
	case p.Online != nil && !*p.Online:
		n.Path = "offline"
	case p.Active != nil && !*p.Active:
		n.Path = "idle"
	}
	return n
}
func parseStatus(r *Report, text string) {
	var status struct {
		BackendState, Version string
		Self                  *statusNode
		Peer                  map[string]*statusNode
	}
	if err := json.Unmarshal([]byte(text), &status); err != nil {
		r.Errors = append(r.Errors, "status JSON: "+err.Error())
		return
	}
	r.Backend = status.BackendState
	r.Version = status.Version
	if status.Self != nil {
		r.Self = node(*status.Self)
		for _, a := range status.Self.Addrs {
			r.Endpoints = append(r.Endpoints, endpoint(a, r.Interfaces, r.Network["dns_iface"]))
		}
	}
	for _, p := range status.Peer {
		if p != nil {
			r.Peers = append(r.Peers, node(*p))
		}
	}
	sort.Slice(r.Peers, func(i, j int) bool { return r.Peers[i].HostName < r.Peers[j].HostName })
}
func optionalBool(value any) *bool {
	var b bool
	switch v := value.(type) {
	case bool:
		b = v
	case string:
		if v != "true" && v != "false" {
			return nil
		}
		b = v == "true"
	default:
		return nil
	}
	return &b
}
func parseNetcheck(r *Report, text string) {
	var values map[string]any
	if err := json.Unmarshal([]byte(text), &values); err != nil {
		r.Errors = append(r.Errors, "netcheck JSON: "+err.Error())
		return
	}
	n := &r.Netcheck
	n.UDP, n.IPv4, n.IPv6 = optionalBool(values["UDP"]), optionalBool(values["IPv4"]), optionalBool(values["IPv6"])
	n.MappingVariesByDestIP, n.CaptivePortal = optionalBool(values["MappingVariesByDestIP"]), optionalBool(values["CaptivePortal"])
	n.PortMapping = map[string]*bool{}
	for _, key := range []string{"UPnP", "PMP", "PCP"} {
		n.PortMapping[key] = optionalBool(values[key])
	}
	if id, ok := values["PreferredDERP"].(float64); ok {
		n.NearestDERP.ID = int(id)
	}
	n.GlobalV4, _ = values["GlobalV4"].(string)
	n.GlobalV6, _ = values["GlobalV6"].(string)
}
func parseDERP(r *Report, text string) {
	var values struct {
		Regions map[string]struct {
			RegionID               int
			RegionCode, RegionName string
		}
	}
	if err := json.Unmarshal([]byte(text), &values); err != nil {
		r.Errors = append(r.Errors, "DERP map JSON: "+err.Error())
		return
	}
	for _, item := range values.Regions {
		region := Region{ID: item.RegionID, Code: item.RegionCode, Name: item.RegionName}
		if r.Self.Relay.Code != "" && r.Self.Relay.Code == item.RegionCode {
			r.Self.Relay = region
		}
		if r.Netcheck.NearestDERP.ID != 0 && r.Netcheck.NearestDERP.ID == item.RegionID {
			r.Netcheck.NearestDERP = region
		}
		for i := range r.Peers {
			if r.Peers[i].Relay.Code != "" && r.Peers[i].Relay.Code == item.RegionCode {
				r.Peers[i].Relay = region
			}
		}
	}
}
func udpListeners(text, pid string) []Endpoint {
	out := []Endpoint{}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		owned := strings.Contains(line, `"tailscaled"`) || pid != "" && strings.Contains(line, "pid="+pid+",")
		if !owned {
			continue
		}
		value := fields[3]
		if strings.HasPrefix(value, "*:") {
			value = "[::]:" + strings.TrimPrefix(value, "*:")
		}
		e := endpoint(value, nil, "")
		if e.Port != 0 {
			out = append(out, e)
		}
	}
	return out
}
func kv(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok && strings.HasPrefix(key, "dns_") {
			out[key] = value
		}
	}
	return out
}
func routeInterface(text string) string {
	words := strings.Fields(text)
	for i := 0; i+1 < len(words); i++ {
		if words[i] == "dev" && physical(words[i+1]) {
			return words[i+1]
		}
	}
	return ""
}
func collect(ctx context.Context, o Options) Report {
	r := Report{Schema: 1, Checked: time.Now().UTC().Format(time.RFC3339), OuterMark: o.Mark, Raw: map[string]CommandResult{}, Peers: []Node{}, Endpoints: []Endpoint{}, Interfaces: interfaces(), UDPListeners: []Endpoint{}, Errors: []string{}, Notes: []string{
		"Self.Relay and peer Relay are home DERP; idle/unknown is not a DERP traffic diagnosis.",
		"Self.Addrs are published candidates, not proof of inbound reachability; Public means address scope only.",
		"netcheck uses a temporary UDP socket; successful STUN is not proof that daemon UDP 41641 is reachable.",
		"A NAT-mapped endpoint's physical interface is an inference unless its IP matches a local physical address.",
		"Commands and output are bounded; errors are diagnostic only. No preferences, routes, DNS or keys are changed.",
	}}
	if data, err := os.ReadFile(filepath.Join(o.Dir, "dns-status")); err == nil && len(data) <= outputLimit {
		r.Network = kv(string(data))
	} else {
		r.Network = map[string]string{}
	}
	r.Network["source"] = "cached dns-status (may be stale)"
	jobs := map[string][]string{
		"status":        append([]string{o.CLI}, append(append([]string{}, o.Args...), "status", "--json")...),
		"netcheck":      append([]string{o.CLI}, append(append([]string{}, o.Args...), "netcheck", "--format=json")...),
		"derp_map":      append([]string{o.CLI}, append(append([]string{}, o.Args...), "debug", "derp-map")...),
		"preferences":   append([]string{o.CLI}, append(append([]string{}, o.Args...), "debug", "prefs")...),
		"magicsock_log": {"tail", "-n", "80", filepath.Join(o.Dir, "run/tailscaled.log")},
		"udp":           {"ss", "-H", "-u", "-l", "-n", "-p"},
		"main_ipv4":     {"ip", "-4", "route", "show", "table", "main"},
		"main_ipv6":     {"ip", "-6", "route", "show", "table", "main"},
		"rules_ipv4":    {"ip", "-4", "rule", "show"},
		"rules_ipv6":    {"ip", "-6", "rule", "show"},
		"outer_ipv4":    {"ip", "-4", "route", "get", "1.1.1.1", "mark", o.Mark},
		"outer_ipv6":    {"ip", "-6", "route", "get", "2606:4700:4700::1111", "mark", o.Mark},
	}
	if o.Export {
		delete(jobs, "magicsock_log")
	}
	if _, err := os.Stat(filepath.Join(o.Dir, "bin/android-dns")); err == nil {
		jobs["android_network"] = []string{filepath.Join(o.Dir, "bin/android-dns"), "--network", "--iface", r.Network["dns_route_hint"]}
	}
	var mutex sync.Mutex
	var wg sync.WaitGroup
	for key, command := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := runCommand(ctx, command[0], command[1:]...)
			mutex.Lock()
			r.Raw[key] = result
			mutex.Unlock()
		}()
	}
	wg.Wait()
	// debug prefs can contain Persist private keys: retain only RouteAll, never
	// the raw response, even when JSON is malformed or truncated.
	prefs := r.Raw["preferences"]
	var safePrefs struct{ RouteAll *bool }
	if prefs.Error == "" && !prefs.Truncated {
		if err := json.Unmarshal([]byte(prefs.Stdout), &safePrefs); err != nil {
			prefs.Error = "unreadable preferences JSON"
		}
	}
	r.AcceptRoutes = safePrefs.RouteAll
	prefs.Stdout = "RouteAll=" + boolText(r.AcceptRoutes)
	prefs.Stderr = "" // never propagate an unsanitized debug-prefs error response
	r.Raw["preferences"] = prefs
	if fresh, ok := r.Raw["android_network"]; ok && fresh.Error == "" && !fresh.Truncated {
		for key, value := range kv(fresh.Stdout) {
			r.Network[key] = value
		}
		r.Network["source"] = "android-dns --network (read-only)"
	}
	if !physical(r.Network["dns_iface"]) {
		if iface := routeInterface(r.Raw["outer_ipv4"].Stdout); iface != "" {
			r.Network["dns_iface"] = iface
			r.Network["source"] = "marked route inference"
		}
	}
	for _, key := range []string{"status", "netcheck", "derp_map"} {
		value := r.Raw[key]
		if value.Error != "" || value.Truncated {
			r.Errors = append(r.Errors, key+": "+value.Error+fmt.Sprintf(" timeout=%t truncated=%t", value.Timeout, value.Truncated))
			continue
		}
		switch key {
		case "status":
			parseStatus(&r, value.Stdout)
		case "netcheck":
			parseNetcheck(&r, value.Stdout)
		case "derp_map":
			parseDERP(&r, value.Stdout)
		}
	}
	r.UDPListeners = udpListeners(r.Raw["udp"].Stdout, o.PID)
	// Validate the actual peer destination, not only a generic route target.
	for i, peer := range r.Peers {
		if i >= 8 || ctx.Err() != nil {
			break
		}
		if address, err := netip.ParseAddrPort(peer.CurAddr); err == nil {
			family := "-6"
			if address.Addr().Is4() {
				family = "-4"
			}
			r.Raw["peer_outer_"+peer.HostName] = runCommand(ctx, "ip", family, "route", "get", address.Addr().String(), "mark", o.Mark)
		}
	}
	if o.Ping {
		target := o.Peer
		if target == "" {
			for _, peer := range r.Peers {
				if peer.Online != nil && *peer.Online && len(peer.IPv4) > 0 {
					target = peer.IPv4[0]
					break
				}
			}
		}
		if ip, err := netip.ParseAddr(target); err == nil {
			args := append(append([]string{}, o.Args...), "ping", "--timeout=2s", "-c", "3", ip.String())
			wg.Add(2)
			go func() {
				defer wg.Done()
				value := runCommand(ctx, o.CLI, args...)
				mutex.Lock()
				r.Raw["tailscale_ping"] = value
				mutex.Unlock()
			}()
			go func() {
				defer wg.Done()
				value := runCommand(ctx, "ping", "-c", "2", "-W", "2", ip.String())
				mutex.Lock()
				r.Raw["kernel_ping"] = value
				mutex.Unlock()
			}()
			wg.Wait()
		} else {
			r.Notes = append(r.Notes, "ping skipped: no online peer IP (or invalid explicit peer)")
		}
	}
	return r
}
func textReport(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "=== Tailscale network diagnostics (%s) ===\nclient version: %s\nbackend state: %s\naccept-routes: %s\nSelf HostName: %s\nSelf OS: %s\nTailnet IPv4: %s\nTailnet IPv6: %s\nHome DERP: %s (%s)\nouter fwmark: %s\n", r.Checked, r.Version, r.Backend, boolText(r.AcceptRoutes), r.Self.HostName, r.Self.OS, strings.Join(r.Self.IPv4, ","), strings.Join(r.Self.IPv6, ","), r.Self.Relay.Code, r.Self.Relay.Name, r.OuterMark)
	for _, e := range r.Endpoints {
		fmt.Fprintf(&b, "endpoint: %s | %s %s | iface=%s (%s)\n", e.Address, e.Family, e.Scope, e.Interface, e.InterfaceSource)
	}
	for _, p := range r.Peers {
		fmt.Fprintf(&b, "peer: %s | path=%s | online=%s active=%s | home DERP=%s (%s) | current=%s | %s %s\n", p.HostName, p.Path, boolText(p.Online), boolText(p.Active), p.Relay.Code, p.Relay.Name, p.CurAddr, strings.Join(p.IPv4, ","), strings.Join(p.IPv6, ","))
	}
	fmt.Fprintf(&b, "netcheck: UDP=%s IPv4=%s IPv6=%s MappingVariesByDestIP=%s NearestDERP=%s (%s)\n", boolText(r.Netcheck.UDP), boolText(r.Netcheck.IPv4), boolText(r.Netcheck.IPv6), boolText(r.Netcheck.MappingVariesByDestIP), r.Netcheck.NearestDERP.Code, r.Netcheck.NearestDERP.Name)
	for _, key := range []string{"UPnP", "PMP", "PCP"} {
		fmt.Fprintf(&b, "PortMapping %s: %s\n", key, boolText(r.Netcheck.PortMapping[key]))
	}
	keys := []string{}
	for key := range r.Network {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%s\n", key, r.Network[key])
	}
	for _, note := range r.Notes {
		fmt.Fprintf(&b, "note: %s\n", note)
	}
	keys = keys[:0]
	for key := range r.Raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := r.Raw[key]
		fmt.Fprintf(&b, "--- raw %s (timeout=%t truncated=%t error=%s) ---\n%s\n%s\n", key, value.Timeout, value.Truncated, value.Error, value.Stdout, value.Stderr)
	}
	for _, err := range r.Errors {
		fmt.Fprintf(&b, "diagnostic error: %s\n", err)
	}
	return b.String()
}
func boolText(value *bool) string {
	if value == nil {
		return "unknown"
	}
	return strconv.FormatBool(*value)
}
func main() {
	o := Options{}
	flag.StringVar(&o.CLI, "cli", "/data/adb/tailscale/bin/tailscale", "installed CLI path")
	flag.StringVar(&o.Dir, "dir", "/data/adb/tailscale", "read-only network status directory")
	flag.StringVar(&o.PID, "pid", "", "daemon PID for UDP listener attribution")
	flag.StringVar(&o.Mark, "mark", "0x10020000", "fwmark for read-only route lookups")
	flag.StringVar(&o.Peer, "peer", "", "optional peer IP for selftest ping")
	flag.BoolVar(&o.Ping, "ping", false, "bounded ping of explicit or first online peer")
	format := flag.String("format", "json", "json, text, report or exit-audit (read-only)")
	watchParent := flag.Int("watch-parent", 0, "optional passive network observer for its watchdog parent")
	timeout := flag.Duration("timeout", 15*time.Second, "total diagnostic deadline (maximum 30s)")
	flag.Parse()
	if *watchParent != 0 {
		if err := watchNetwork(*watchParent, o.Dir); err != nil {
			fmt.Fprintln(os.Stderr, "network-observer unavailable:", err)
			os.Exit(1)
		}
		return
	}
	o.Args = flag.Args()
	if _, err := strconv.ParseUint(o.Mark, 0, 32); err != nil || *timeout <= 0 || *timeout > 30*time.Second || (*format != "json" && *format != "text" && *format != "report" && *format != "exit-audit") {
		fmt.Fprintln(os.Stderr, "invalid diagnostic options")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if *format == "report" || *format == "exit-audit" {
		o.ExitAudit = *format == "exit-audit"
		fmt.Print(diagnosticExport(ctx, o))
		return
	}
	r := collect(ctx, o)
	if *format == "text" {
		fmt.Print(textReport(r))
	} else {
		json.NewEncoder(os.Stdout).Encode(r)
	}
}
