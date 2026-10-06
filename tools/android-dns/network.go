package main

import (
	"context"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

// Selection contains only the parent LinkProperties of a physical network.
// RouteHint is diagnostic input, never authority over ConnectivityService.
type Selection struct {
	ID, Iface, Transport, VPN, Underlying, Excluded, Reason, Route string
	DNS                                                            []string
}

type androidNetwork struct {
	id, iface, transport, route string
	dns, underlying, interfaces []string
	vpn, declared, connected    bool
}

var dnsList = regexp.MustCompile(`DnsAddresses:\s*\[([^\]]*)\]`)
var ifaceName = regexp.MustCompile(`InterfaceName:\s*([^\s,}]+)`)
var networkID = regexp.MustCompile(`network\{([0-9]+)\}`)
var activeID = regexp.MustCompile(`Active default network:\s*([0-9]+)`)
var agentStart = regexp.MustCompile(`(?m)^\s*(?:NetworkAgentInfo|networkAgentInfo)\s*[\[{]`)
var transportList = regexp.MustCompile(`Transports:\s*([A-Z_]+(?:[|&][A-Z_]+)*)`)
var underlyingList = regexp.MustCompile(`(?i)(?:underlyingNetworks:\s*|underlying\{\s*)(Null|\[[^\]]*\])`)
var decimalID = regexp.MustCompile(`[0-9]+`)
var defaultV4 = regexp.MustCompile(`0\.0\.0\.0/0\s*->\s*([0-9.]+)\s+([^\s,\]]+)`)
var linkAddresses = regexp.MustCompile(`LinkAddresses:\s*\[([^\]]*)\]`)

func virtualInterface(name string) bool {
	name = strings.ToLower(name)
	name = strings.TrimPrefix(name, "v4-")
	for _, prefix := range []string{"tun", "tap", "tailscale", "wg", "ppp", "ipsec", "dummy", "veth"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return name == "" || name == "lo"
}

func parseNetworks(dump string) (string, []androidNetwork) {
	active := ""
	if m := activeID.FindStringSubmatch(dump); len(m) > 1 {
		active = m[1]
	}
	if i := strings.Index(dump, "Current Networks:"); i >= 0 {
		dump = dump[i:]
	}
	if i := strings.Index(dump, "Network Requests:"); i >= 0 {
		dump = dump[:i]
	}
	starts := agentStart.FindAllStringIndex(dump, -1)
	var out []androidNetwork
	for i, pos := range starts {
		end := len(dump)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		block := dump[pos[0]:end]
		// Requests attached to each agent also contain capabilities/underlying IDs.
		// They are not properties of the NetworkAgentInfo itself.
		if j := strings.Index(block, "\n    Requests:"); j >= 0 {
			block = block[:j]
		}
		id, name := networkID.FindStringSubmatch(block), ifaceName.FindStringSubmatch(block)
		if len(id) < 2 || len(name) < 2 {
			continue
		}
		n := androidNetwork{id: id[1], iface: name[1], connected: !strings.Contains(block, "DISCONNECTED") && !strings.Contains(block, "SUSPENDED")}
		// NOT_SUSPENDED is a capability, not a disconnected NetworkInfo state.
		n.connected = !regexp.MustCompile(`ni\{[^}]*\b(?:DISCONNECTED|SUSPENDED)\b`).MatchString(block)
		for _, m := range ifaceName.FindAllStringSubmatch(block, -1) {
			n.interfaces = append(n.interfaces, m[1])
		}
		if m := transportList.FindStringSubmatch(block); len(m) > 1 {
			n.transport = m[1]
		}
		n.vpn = regexp.MustCompile(`ni\{VPN\b|\btype:\s*VPN\b`).MatchString(block)
		for _, transport := range strings.FieldsFunc(n.transport, func(r rune) bool { return r == '|' || r == '&' }) {
			n.vpn = n.vpn || transport == "VPN"
		}
		// Prefer declared underlying{} on older releases over derived capabilities.
		for _, m := range underlyingList.FindAllStringSubmatch(block, -1) {
			if strings.EqualFold(m[1], "Null") {
				continue
			}
			n.declared = true
			n.underlying = decimalID.FindAllString(m[1], -1)
			break
		}
		if m := dnsList.FindStringSubmatch(block); len(m) > 1 {
			n.dns = validServers(strings.FieldsFunc(m[1], func(r rune) bool { return r == ',' || r == '/' || r == ' ' }), n.iface)
		}
		for _, m := range defaultV4.FindAllStringSubmatch(block, -1) {
			belongs := false
			for _, iface := range n.interfaces {
				belongs = belongs || m[2] == iface
			}
			if !belongs || virtualInterface(m[2]) {
				continue
			}
			gw, src := m[1], ""
			if gw == "0.0.0.0" {
				gw = "-"
			}
			src = ipv4Source(block, m[2])
			n.route = fmt.Sprintf("%s %s %s", gw, m[2], src)
			break
		}
		out = append(out, n)
	}
	return active, out
}

func ipv4Source(block, name string) string {
	positions := ifaceName.FindAllStringSubmatchIndex(block, -1)
	for i, pos := range positions {
		if block[pos[2]:pos[3]] != name {
			continue
		}
		end := len(block)
		if i+1 < len(positions) {
			end = positions[i+1][0]
		}
		if a := linkAddresses.FindStringSubmatch(block[pos[0]:end]); len(a) > 1 {
			for _, item := range strings.FieldsFunc(a[1], func(r rune) bool { return r == ',' || r == ' ' }) {
				if p, err := netip.ParsePrefix(item); err == nil && p.Addr().Is4() {
					return p.Addr().String()
				}
			}
		}
	}
	return ""
}

func selectNetwork(dump, hint string) Selection {
	active, networks := parseNetworks(dump)
	s := Selection{Reason: "no-physical-network"}
	byID := map[string]androidNetwork{}
	var vpns []androidNetwork
	var excluded []string
	for _, n := range networks {
		byID[n.id] = n
		if n.vpn || virtualInterface(n.iface) {
			excluded = append(excluded, n.id+"/"+n.iface+"/"+n.transport)
			if n.vpn && n.connected {
				vpns = append(vpns, n)
			}
		}
	}
	if virtualInterface(hint) && hint != "" {
		excluded = append(excluded, "route-hint/"+hint)
	}
	s.Excluded = strings.Join(excluded, ",")
	var walk func(string, map[string]bool) (androidNetwork, bool)
	walk = func(id string, seen map[string]bool) (androidNetwork, bool) {
		n, exists := byID[id]
		if !exists || seen[id] || !n.connected {
			return androidNetwork{}, false
		}
		seen[id] = true
		if n.vpn {
			if n.declared {
				for _, child := range n.underlying {
					if p, ok := walk(child, seen); ok {
						return p, true
					}
				}
				return androidNetwork{}, false
			}
			if active != id {
				return walk(active, seen)
			}
			return androidNetwork{}, false
		}
		if virtualInterface(n.iface) {
			return androidNetwork{}, false
		}
		return n, true
	}
	finish := func(n androidNetwork, reason string) Selection {
		s.ID, s.Iface, s.Transport, s.DNS, s.Route, s.Reason = n.id, n.iface, n.transport, n.dns, n.route, reason
		return s
	}
	// Match the active VPN or the VPN owning the route hint. With one live VPN,
	// Android's active default may still be its physical network (as on Redmi).
	var vpn *androidNetwork
	for i := range vpns {
		if vpns[i].id == active {
			vpn = &vpns[i]
			break
		}
	}
	if vpn == nil {
		for i := range vpns {
			if vpns[i].iface == hint {
				vpn = &vpns[i]
				break
			}
		}
	}
	if vpn == nil && len(vpns) == 1 {
		vpn = &vpns[0]
	}
	if vpn != nil {
		s.VPN, s.Underlying = vpn.id, strings.Join(vpn.underlying, ",")
		if !vpn.declared {
			s.Underlying = "default"
		} else if len(vpn.underlying) == 0 {
			s.Underlying = "none"
		}
		if n, ok := walk(vpn.id, map[string]bool{}); ok {
			return finish(n, "vpn-underlying-linkproperties")
		}
		s.Reason = "vpn-underlying-unavailable"
		return s // Explicit [] or missing IDs must not select an unrelated IMS/Wi-Fi.
	}
	if n, ok := walk(active, map[string]bool{}); ok {
		return finish(n, "active-default-linkproperties")
	}
	// Only old dumps lacking an active ID may use a physical route hint.
	if active == "" && !virtualInterface(hint) {
		for _, n := range networks {
			for _, iface := range n.interfaces {
				if iface == hint && !n.vpn && n.connected && !virtualInterface(n.iface) {
					return finish(n, "physical-route-hint")
				}
			}
		}
	}
	return s
}

func connectivityDNS(dump, iface string) []string { return selectNetwork(dump, iface).DNS }

func discover(ctx context.Context, iface string) (Selection, []string, string) {
	s := selectNetwork(command(ctx, "dumpsys", "connectivity"), iface)
	if len(s.DNS) > 0 {
		return s, s.DNS, "android-linkproperties"
	}
	// One bounded getprop dump replaces eight sequential processes; discovery
	// has its own budget and can never consume the later DNS probe budget.
	props := command(ctx, "getprop")
	values := map[string]string{}
	for _, line := range strings.Split(props, "\n") {
		if m := regexp.MustCompile(`^\[([^\]]+)\]: \[([^\]]*)\]`).FindStringSubmatch(strings.TrimSpace(line)); len(m) > 2 {
			values[m[1]] = m[2]
		}
	}
	var servers []string
	if s.Iface != "" {
		for _, n := range []string{"1", "2", "3", "4"} {
			servers = append(servers, values["net."+s.Iface+".dns"+n])
		}
		if dns := validServers(servers, s.Iface); len(dns) > 0 {
			return s, dns, "android-interface-property"
		}
	}
	// Global properties can describe the VPN; never trust them with a VPN active
	// or a virtual route hint. The public fallback is probed with the daemon mark.
	if s.VPN == "" && !virtualInterface(iface) {
		servers = nil
		for _, n := range []string{"1", "2", "3", "4"} {
			servers = append(servers, values["net.dns"+n])
		}
		return s, validServers(servers, s.Iface), "android-legacy-property"
	}
	return s, nil, "android-network-unavailable"
}
