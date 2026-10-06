package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStatusPathsDoNotConfuseHomeDERPWithTraffic(t *testing.T) {
	r := Report{}
	parseStatus(&r, `{"Self":{"HostName":"phone","OS":"linux","TailscaleIPs":["100.1.2.3"],"Relay":"hkg","Addrs":["192.168.1.4:41641"]},"Peer":{"a":{"HostName":"offline","Online":false,"CurAddr":"1.2.3.4:41641","Relay":"hkg"},"b":{"HostName":"idle","Online":true,"Active":false,"Relay":"hkg"},"c":{"HostName":"direct","Online":true,"Active":true,"CurAddr":"[2001:db8::2]:41641","Relay":"hkg"},"d":{"HostName":"derp","Online":true,"Active":true,"Relay":"hkg"},"e":{"HostName":"peer-relay","Online":true,"Active":true,"PeerRelay":"1.2.3.4:41641:7","Relay":"hkg"}}}`)
	got := map[string]string{}
	for _, p := range r.Peers {
		got[p.HostName] = p.Path
	}
	for name, expected := range map[string]string{"offline": "offline", "idle": "idle", "direct": "direct", "derp": "derp", "peer-relay": "peer-relay"} {
		if got[name] != expected {
			t.Fatalf("%s path=%q; want %q", name, got[name], expected)
		}
	}
	if r.Self.HostName != "phone" || len(r.Self.IPv6) != 0 || r.Self.Relay.Code != "hkg" {
		t.Fatalf("self: %+v", r.Self)
	}
}

func TestMissingStatusAndUnknownOnlineStayUnknown(t *testing.T) {
	for _, input := range []string{`{}`, `{"Self":null,"Peer":null}`, `{"Self":{},"Peer":{"x":{}}}`} {
		r := Report{}
		parseStatus(&r, input)
		if r.Self.HostName != "" || r.Self.Relay.Code != "" {
			t.Fatal(r.Self)
		}
		for _, p := range r.Peers {
			if p.Path != "unknown" || p.Online != nil {
				t.Fatal(p)
			}
		}
	}
	r := Report{}
	parseStatus(&r, `{"Self":`)
	if len(r.Errors) == 0 {
		t.Fatal("malformed JSON must have an explicit error")
	}
}

func TestControlOfflineDoesNotHideActiveDataPlane(t *testing.T) {
	r := Report{}
	parseStatus(&r, `{"Peer":{"a":{"HostName":"active-direct","Online":false,"Active":true,"CurAddr":"1.2.3.4:41641"},"b":{"HostName":"active-relay","Online":false,"Active":true,"Relay":"hkg"}}}`)
	for _, p := range r.Peers {
		want := "direct"
		if p.HostName == "active-relay" {
			want = "derp"
		}
		if p.Path != want || p.Online == nil || *p.Online {
			t.Fatalf("control/data-plane conflated: %+v", p)
		}
	}
}

func TestEndpointsClassifyAddressesAndDoNotInventNATInterfaces(t *testing.T) {
	interfaces := []Interface{{Name: "wlan0", Addresses: []string{"192.168.1.4", "2001:db8::4"}}, {Name: "tailscale0", Addresses: []string{"100.1.2.3"}}}
	for _, tc := range []struct{ address, family, scope, iface string }{
		{"192.168.1.4:41641", "IPv4", "Private", "wlan0"},
		{"[2001:db8::4]:41641", "IPv6", "Public", "wlan0"},
		{"[fd00::4]:41641", "IPv6", "Private", ""},
		{"100.64.1.2:5555", "IPv4", "CGNAT", ""},
		{"203.0.113.4:54321", "IPv4", "Public", ""},
		{"[fe80::4%wlan0]:41641", "IPv6", "Link-local", "wlan0"},
		{"127.0.0.1:41641", "IPv4", "Loopback", ""},
	} {
		e := endpoint(tc.address, interfaces, "")
		if e.Family != tc.family || e.Scope != tc.scope || e.Interface != tc.iface {
			t.Fatalf("%s: %+v", tc.address, e)
		}
	}
	e := endpoint("203.0.113.4:54321", interfaces, "wlan0")
	if e.Interface != "wlan0" || !strings.Contains(e.InterfaceSource, "inferred") {
		t.Fatal(e)
	}
	if endpoint("garbage", interfaces, "wlan0").Family != "Unknown" {
		t.Fatal("invalid endpoint")
	}
	if endpoint("100.1.2.3:41641", interfaces, "").Interface != "" {
		t.Fatal("tunnel is not a physical interface")
	}
}

func TestNetcheckUsesPreferredDERPAndUnknownOptionalBooleans(t *testing.T) {
	r := Report{}
	parseNetcheck(&r, `{"UDP":true,"IPv4":false,"IPv6":true,"MappingVariesByDestIP":"","UPnP":"false","PMP":"true","PreferredDERP":1,"GlobalV6":"[2001:db8::5]:12345"}`)
	parseDERP(&r, `{"Regions":{"1":{"RegionID":1,"RegionCode":"hkg","RegionName":"Hong Kong"}}}`)
	if r.Netcheck.UDP == nil || !*r.Netcheck.UDP || r.Netcheck.IPv4 == nil || *r.Netcheck.IPv4 || r.Netcheck.MappingVariesByDestIP != nil {
		t.Fatal(r.Netcheck)
	}
	if r.Netcheck.NearestDERP.Code != "hkg" || r.Netcheck.NearestDERP.Name != "Hong Kong" || r.Netcheck.PortMapping["PMP"] == nil || !*r.Netcheck.PortMapping["PMP"] {
		t.Fatal(r.Netcheck)
	}
	r = Report{}
	parseNetcheck(&r, `{}`)
	parseDERP(&r, `{}`)
	if r.Netcheck.UDP != nil || r.Netcheck.NearestDERP.Name != "" {
		t.Fatal(r.Netcheck)
	}
}

func TestCommandTimeoutIsBoundedAndOutputIsLimited(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	start := time.Now()
	r := runCommand(ctx, "/bin/sleep", "5")
	if !r.Timeout || time.Since(start) > time.Second {
		t.Fatalf("timeout: %+v", r)
	}
	r = runCommand(context.Background(), "/bin/sh", "-c", "yes x | head -c 300000")
	if !r.Truncated || len(r.Stdout) > outputLimit {
		t.Fatalf("unbounded output: %d", len(r.Stdout))
	}
}

func TestCollectionFailureDoesNotWriteStateOrPublishFalseSuccess(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "tailscaled.state")
	os.WriteFile(state, []byte("private identity"), 0600)
	cli := filepath.Join(dir, "tailscale")
	os.WriteFile(cli, []byte("#!/bin/sh\nexec sleep 10\n"), 0700)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	r := collect(ctx, Options{CLI: cli, Dir: dir, Mark: "0x10020000"})
	if !r.Raw["status"].Timeout || r.Netcheck.UDP != nil {
		t.Fatal(r)
	}
	data, _ := os.ReadFile(state)
	mode, _ := os.Stat(state)
	if string(data) != "private identity" || mode.Mode().Perm() != 0600 {
		t.Fatal("identity modified")
	}
	if _, err := json.Marshal(r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(textReport(r), "netcheck") {
		t.Fatal("missing readable diagnostics on failure")
	}
}

func TestUDPListenerDistinguishesOtherServicesAndPort(t *testing.T) {
	got := udpListeners("UNCONN 0 0 0.0.0.0:41641 0.0.0.0:* users:((\"tailscaled\",pid=123,fd=9))\nUNCONN 0 0 [::]:41641 [::]:* users:((\"tailscaled\",pid=123,fd=10))\nUNCONN 0 0 127.0.0.1:7890 0.0.0.0:* users:((\"mihomo\",pid=444,fd=7))", "123")
	if len(got) != 2 || got[0].Port != 41641 || got[1].Family != "IPv6" {
		t.Fatal(got)
	}
}

func TestCollectionPreservesSocketAndNeverCopiesPrefsSecrets(t *testing.T) {
	dir := t.TempDir()
	cli := filepath.Join(dir, "tailscale")
	script := `#!/bin/sh
[ "$1" = "--socket=/custom/socket" ] || exit 9
shift
case "$*" in
 "status --json") echo '{"Version":"1.102.5","Self":{"HostName":"phone"},"Peer":{"p":{"Online":false,"TailscaleIPs":["100.1.2.3"]}}}' ;;
 "netcheck --format=json") echo '{"UDP":false,"IPv6":false}' ;;
 "debug derp-map") echo '{}' ;;
 "debug prefs") echo '{"RouteAll":true,"Persist":{"PrivateNodeKey":"NEVER_COPY_SECRET"}}'; echo NEVER_COPY_SECRET >&2 ;;
 *) exit 8 ;;
esac
`
	os.WriteFile(cli, []byte(script), 0700)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r := collect(ctx, Options{CLI: cli, Dir: dir, Mark: "0x10020000", Args: []string{"--socket=/custom/socket"}, Ping: true})
	data, _ := json.Marshal(r)
	if r.Self.HostName != "phone" || r.AcceptRoutes == nil || !*r.AcceptRoutes {
		t.Fatal(r.Errors)
	}
	if strings.Contains(string(data), "NEVER_COPY_SECRET") || strings.Contains(textReport(r), "NEVER_COPY_SECRET") {
		t.Fatal("prefs private data exposed")
	}
	if _, ok := r.Raw["tailscale_ping"]; ok {
		t.Fatal("offline peer pinged")
	}
}
