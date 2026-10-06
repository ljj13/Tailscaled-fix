package main

import (
	"strings"
	"testing"
)

const vpnDump = `Active default network: 106
Current Networks:
 NetworkAgentInfo{network{100} ni{MOBILE CONNECTED extra: ims} lp{{InterfaceName: ccmni0 DnsAddresses: [/10.9.0.1]}} nc{[Transports: CELLULAR Capabilities: IMS&NOT_VPN UnderlyingNetworks: Null]}}
 NetworkAgentInfo{network{106} ni{MOBILE CONNECTED} lp{{InterfaceName: ccmni1 LinkAddresses: [10.166.186.75/32] DnsAddresses: [/120.80.80.80,/221.5.88.88] Routes: [0.0.0.0/0 -> 10.166.186.75 ccmni1 mtu 1400]}} nc{[Transports: CELLULAR Capabilities: INTERNET&NOT_VPN UnderlyingNetworks: Null]}}
 NetworkAgentInfo{network{107} ni{VPN CONNECTED} lp{{InterfaceName: tun0 DnsAddresses: [/172.19.0.2]}} nc{[Transports: CELLULAR|VPN Capabilities: INTERNET UnderlyingNetworks: [106]]}}
Network Requests:
 NetworkAgentInfo{network{900} lp{{InterfaceName: bad DnsAddresses: [/10.99.0.1]}}}`

func TestVPNUnderlyingOverridesOrdinaryRoute(t *testing.T) {
	for _, hint := range []string{"tun0", "", "ccmni0", "tailscale0"} {
		s := selectNetwork(vpnDump, hint)
		if s.ID != "106" || s.Iface != "ccmni1" || s.Transport != "CELLULAR" || strings.Join(s.DNS, ",") != "120.80.80.80,221.5.88.88" || s.Route != "10.166.186.75 ccmni1 10.166.186.75" {
			t.Fatalf("hint %q: %+v", hint, s)
		}
		if !strings.Contains(s.Excluded, "107/tun0") || s.VPN != "107" || !strings.Contains(s.Reason, "vpn-underlying") {
			t.Fatalf("missing diagnosis: %+v", s)
		}
	}
}

func TestVPNNullEmptyAndMissingUnderlying(t *testing.T) {
	for _, tc := range []struct{ value, id string }{{"Null", "106"}, {"[]", ""}, {"[999]", ""}, {"[107]", ""}, {"[999,106]", "106"}} {
		s := selectNetwork(strings.Replace(vpnDump, "UnderlyingNetworks: [106]", "UnderlyingNetworks: "+tc.value, 1), "tun0")
		if s.ID != tc.id {
			t.Fatalf("%s: %+v", tc.value, s)
		}
	}
}

func TestExcludeVirtualEvenWithoutVPNTransport(t *testing.T) {
	for _, iface := range []string{"tun0", "tun5", "tailscale0", "wg0", "ppp0"} {
		d := "Active default network: 1\nCurrent Networks:\n NetworkAgentInfo{network{1} ni{WIFI CONNECTED} lp{{InterfaceName: " + iface + " DnsAddresses: [/10.1.1.1]}} nc{[Transports: WIFI]}}"
		if s := selectNetwork(d, iface); s.ID != "" {
			t.Fatalf("virtual selected: %+v", s)
		}
	}
}

func TestLegacyDeclaredUnderlyingAndActiveVPN(t *testing.T) {
	d := strings.Replace(vpnDump, "Active default network: 106", "Active default network: 107", 1)
	d = strings.Replace(d, "UnderlyingNetworks: [106]", "UnderlyingNetworks: Null", 1)
	d = strings.Replace(d, "ni{VPN CONNECTED}", "ni{VPN CONNECTED} underlying{[106]}", 1)
	if s := selectNetwork(d, "tun0"); s.ID != "106" {
		t.Fatalf("legacy: %+v", s)
	}
}

func TestSelectedCLATRouteBelongsToParentNetwork(t *testing.T) {
	d := `Active default network: 106
Current Networks:
 NetworkAgentInfo{network{106} ni{MOBILE CONNECTED} lp{{InterfaceName: ccmni1 LinkAddresses: [2001:db8::2/64] DnsAddresses: [/10.1.1.1] Routes: [::/0 -> :: ccmni1 mtu 0] Stacked: [{InterfaceName: v4-ccmni1 LinkAddresses: [192.0.0.4/32] DnsAddresses: [] Routes: [0.0.0.0/0 -> 0.0.0.0 v4-ccmni1 mtu 0]}]}}}`
	s := selectNetwork(d, "v4-ccmni1")
	if s.Iface != "ccmni1" || s.Route != "- v4-ccmni1 192.0.0.4" {
		t.Fatalf("CLAT: %+v", s)
	}
}

func TestActiveVPNHasPriorityOverOtherVPNRouteHint(t *testing.T) {
	d := strings.Replace(vpnDump, "Active default network: 106", "Active default network: 109", 1)
	d = strings.Replace(d, "Network Requests:", ` NetworkAgentInfo{network{109} ni{VPN CONNECTED} lp{{InterfaceName: tun1 DnsAddresses: [/10.99.0.1]}} nc{[Transports: WIFI|VPN UnderlyingNetworks: [110]]}}
 NetworkAgentInfo{network{110} ni{WIFI CONNECTED} lp{{InterfaceName: wlan0 DnsAddresses: [/192.168.1.1]}} nc{[Transports: WIFI]}}
Network Requests:`, 1)
	if s := selectNetwork(d, "tun0"); s.VPN != "109" || s.ID != "110" || s.Iface != "wlan0" {
		t.Fatalf("hint outranked active VPN: %+v", s)
	}
}
