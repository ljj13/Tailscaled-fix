package main

import (
	"testing"
	"time"
)

func TestPhysicalFingerprintNoiseAndPolicy(t *testing.T) {
	a := "dns_network=108\ndns_iface=ccmni3\ndns_active_vpn=109\ndns_underlying=108\n"
	b := "dns_network=108\ndns_iface=ccmni3\ndns_active_vpn=\ndns_underlying=\n"
	r := "default via fe80::1 dev ccmni3 table ccmni3 proto ra expires 120sec\nfd7a::/48 dev tailscale0 table 52\n"
	f, ok := physicalFingerprint(a, []string{"2408::1", "fe80::2"}, r)
	if !ok {
		t.Fatal("valid physical rejected")
	}
	g, ok := physicalFingerprint(b, []string{"fe80::2", "2408::1"}, "default via fe80::1 dev ccmni3 table ccmni3 proto ra expires 119sec\n")
	if !ok || f != g {
		t.Fatal("VPN, lifetime or TUN noise changed fingerprint")
	}
	for _, network := range []string{"", "dns_network=108\ndns_iface=tun0", "dns_network=bad\ndns_iface=wlan0", "dns_network=1\ndns_iface=tailscale0"} {
		if _, ok := physicalFingerprint(network, nil, ""); ok {
			t.Fatal("invalid physical accepted", network)
		}
	}
	for _, changed := range []string{"dns_network=110\ndns_iface=ccmni3", "dns_network=108\ndns_iface=wlan0"} {
		g, _ := physicalFingerprint(changed, []string{"2408::1"}, r)
		if g == f {
			t.Fatal("physical change lost")
		}
	}
	g, _ = physicalFingerprint(a, []string{"2408::2"}, r)
	if g == f {
		t.Fatal("address change lost")
	}
	g, _ = physicalFingerprint(a, []string{"2408::1"}, "")
	if g == f {
		t.Fatal("IPv6 policy removal lost")
	}
}

func TestEventGateDebounceAndBoundedBurst(t *testing.T) {
	base := time.Unix(100, 0)
	g := eventGate{}
	g.event(base)
	if g.ready(base.Add(749 * time.Millisecond)) {
		t.Fatal("not debounced")
	}
	g.event(base.Add(500 * time.Millisecond))
	if g.ready(base.Add(time.Second)) {
		t.Fatal("burst not coalesced")
	}
	if !g.ready(base.Add(1250 * time.Millisecond)) {
		t.Fatal("settled event lost")
	}
	g.checked(base.Add(1250*time.Millisecond), true)
	if g.ready(base.Add(10 * time.Second)) {
		t.Fatal("idle busy polling")
	}
	g.event(base.Add(1300 * time.Millisecond))
	if g.ready(base.Add(2100 * time.Millisecond)) {
		t.Fatal("cooldown lost")
	}
	if !g.ready(base.Add(3250 * time.Millisecond)) {
		t.Fatal("bounded retry lost")
	}
	g.checked(base.Add(3250*time.Millisecond), false)
	if !g.ready(base.Add(5250 * time.Millisecond)) {
		t.Fatal("unavailable discovery never retried")
	}
	g.checked(base.Add(5250*time.Millisecond), true)
	for i := 0; i < 20; i++ {
		g.event(base.Add(6*time.Second + time.Duration(i)*100*time.Millisecond))
	}
	if !g.ready(base.Add(8 * time.Second)) {
		t.Fatal("continuous events starved check")
	}
}

func TestWatchParentMustBeActualParent(t *testing.T) {
	if validWatchParent(1, 1) || validWatchParent(123, 124) || !validWatchParent(123, 123) {
		t.Fatal("unsafe parent accepted")
	}
}

func TestUnavailableDiscoveryRetryBudget(t *testing.T) {
	g := eventGate{}
	now := time.Unix(100, 0)
	g.event(now)
	g.checked(now.Add(10*time.Second), false)
	if g.ready(now.Add(30 * time.Second)) {
		t.Fatal("unavailable Android caused endless fast polling")
	}
	g.event(now.Add(31 * time.Second))
	if !g.ready(now.Add(32 * time.Second)) {
		t.Fatal("later network event did not recover")
	}
}
