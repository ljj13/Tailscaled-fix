package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetainVerifiedOnlyOnSameUnderlyingNetwork(t *testing.T) {
	now := time.Now()
	old := verifiedDNS{Network: "106", Iface: "ccmni1", Servers: []string{"120.80.80.80"}, Source: "android-linkproperties", Verified: now}
	for _, tc := range []struct {
		s    Selection
		want bool
	}{
		{Selection{ID: "106", Iface: "ccmni1"}, true},
		{Selection{ID: "108", Iface: "ccmni1"}, false},
		{Selection{ID: "106", Iface: "wlan0"}, false},
		{Selection{Reason: "vpn-underlying-unavailable", VPN: "107", Underlying: "106"}, true},
		{Selection{Reason: "vpn-underlying-unavailable", VPN: "107", Underlying: "108"}, false},
		{Selection{Reason: "vpn-underlying-unavailable", VPN: "107", Underlying: "none"}, false},
		{Selection{Reason: "no-physical-network"}, true},
	} {
		if got := canRetain(old, tc.s, now); got != tc.want {
			t.Fatalf("%+v: %v", tc.s, got)
		}
	}
	// Unknown discovery is a short grace period; it cannot pin old DNS forever.
	if canRetain(old, Selection{}, now.Add(3*time.Minute)) {
		t.Fatal("expired unknown-network cache retained")
	}
	if !canRetain(old, Selection{ID: "106", Iface: "ccmni1"}, now.Add(time.Hour)) {
		t.Fatal("known same network must retain last verified resolver")
	}
}

func TestFailedRefreshRetainsVerifiedResolverInsteadOfPublicUnverified(t *testing.T) {
	old := verifiedDNS{Network: "106", Iface: "ccmni1", Source: "android-linkproperties", Servers: []string{"120.80.80.80"}, Verified: time.Now()}
	fail := func(string) error { return os.ErrDeadlineExceeded }
	same := Selection{ID: "106", Iface: "ccmni1"}
	servers, source, ok, retained, _ := chooseCached(nil, "android-network-unavailable", []string{"1.1.1.1"}, old, canRetain(old, same, time.Now()), fail)
	if len(servers) != 1 || servers[0] != old.Servers[0] || source != old.Source || ok || !retained {
		t.Fatalf("%v %s %v %v", servers, source, ok, retained)
	}
	changed := Selection{ID: "109", Iface: "wlan0"}
	servers, source, _, retained, _ = chooseCached([]string{"192.168.1.1"}, "android-linkproperties", []string{"1.1.1.1"}, old, canRetain(old, changed, time.Now()), fail)
	if servers[0] != "192.168.1.1" || retained || source != "android-linkproperties-unverified" {
		t.Fatal("old network leaked", servers, source, retained)
	}
}

func TestVerifiedCacheInvalidatesAcrossReboots(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dns-verified.json")
	old := verifiedDNS{BootID: "different-boot", Network: "106", Iface: "ccmni1", Servers: []string{"120.80.80.80"}, Verified: time.Now()}
	data, _ := json.Marshal(old)
	os.WriteFile(path, data, 0600)
	if got := readVerified(path); len(got.Servers) > 0 {
		t.Fatal("previous boot cache reused", got)
	}
	old.BootID = bootID()
	data, _ = json.Marshal(old)
	os.WriteFile(path, data, 0600)
	if got := readVerified(path); len(got.Servers) != 1 {
		t.Fatal("same boot cache lost", got)
	}
}

func TestDisablingFallbackRevokesPublishedPublicDNSWithoutTouchingState(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bootstrap-resolv.conf"), []byte("nameserver 1.1.1.1\n"), 0600)
	os.WriteFile(filepath.Join(dir, "dns-status"), []byte("dns_source=public-fallback\n"), 0600)
	os.WriteFile(filepath.Join(dir, "tailscaled.state"), []byte("node identity"), 0600)
	if err := revokePublicBootstrap(dir); err != nil {
		t.Fatal(err)
	}
	if got := publishedServers(dir); len(got) != 0 {
		t.Fatal("revoked DNS still published", got)
	}
	state, _ := os.ReadFile(filepath.Join(dir, "tailscaled.state"))
	if string(state) != "node identity" {
		t.Fatal("state changed")
	}
	os.WriteFile(filepath.Join(dir, "bootstrap-resolv.conf"), []byte("nameserver 120.80.80.80\n"), 0600)
	os.WriteFile(filepath.Join(dir, "dns-status"), []byte("dns_source=android-linkproperties\n"), 0600)
	if err := revokePublicBootstrap(dir); err != nil {
		t.Fatal(err)
	}
	if got := publishedServers(dir); len(got) != 1 || got[0] != "120.80.80.80" {
		t.Fatal("Android resolver revoked", got)
	}
}

func TestDisabledPublicFallbackInvalidatesPublicCache(t *testing.T) {
	old := verifiedDNS{Source: "public-fallback", Servers: []string{"1.1.1.1", "8.8.8.8"}}
	if got := allowedCache(old, nil); len(got.Servers) != 0 {
		t.Fatal("disabled fallback retained", got)
	}
	if got := allowedCache(old, []string{"8.8.8.8"}); len(got.Servers) != 1 || got.Servers[0] != "8.8.8.8" {
		t.Fatal(got)
	}
	old.Source = "android-linkproperties"
	if got := allowedCache(old, nil); len(got.Servers) != 2 {
		t.Fatal("Android DNS cache incorrectly disabled")
	}
}

func TestProbeBudgetIndependentOfExpiredDiscovery(t *testing.T) {
	// Regression: the old shared 18-second context canceled all subsequent probes.
	ctx, cancel := freshProbeContext()
	defer cancel()
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) < 7*time.Second {
		t.Fatal("missing fresh probe budget")
	}
}
