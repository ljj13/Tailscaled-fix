package main

import (
	"context"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectivitySelectsActivePhysicalNetwork(t *testing.T) {
	dump := `Active default network: 101
Current Networks:
  NetworkAgentInfo{network{100} ni{WIFI} lp{{InterfaceName: wlan0 DnsAddresses: [ /192.168.1.1 ]}}}
  NetworkAgentInfo{network{101} ni{MOBILE} lp{{InterfaceName: rmnet_data0 DnsAddresses: [ /2001:4860:4860::8888,/10.0.0.1 ]}}}
  NetworkAgentInfo{network{102} ni{VPN} lp{{InterfaceName: tun0 DnsAddresses: [ /127.0.0.1 ]}}}`
	got := connectivityDNS(dump, "rmnet_data0")
	if strings.Join(got, ",") != "2001:4860:4860::8888,10.0.0.1" {
		t.Fatalf("wrong active DNS: %v", got)
	}
	if got := connectivityDNS(dump, "wlan0"); strings.Join(got, ",") != "2001:4860:4860::8888,10.0.0.1" {
		t.Fatalf("Android active default must win over stale route hint: %v", got)
	}
	if got := connectivityDNS(dump, ""); strings.Join(got, ",") != "2001:4860:4860::8888,10.0.0.1" {
		t.Fatalf("active default not recognized: %v", got)
	}
}

func TestAndroidOlderMultilineAndHistory(t *testing.T) {
	dump := `Active default network: 100
Current Networks:
  NetworkAgentInfo [WIFI () - 100]
    network{100} lp{InterfaceName: wlan0 DnsAddresses: [192.168.8.1,fe80::1]}
Network Requests:
  NetworkAgentInfo{network{99} lp{InterfaceName: wlan0 DnsAddresses: [10.9.0.1]}}`
	got := connectivityDNS(dump, "wlan0")
	if strings.Join(got, ",") != "192.168.8.1,fe80::1%wlan0" {
		t.Fatalf("%v", got)
	}
}

func TestRejectUnsafeAndDeduplicate(t *testing.T) {
	got := validServers([]string{"127.0.0.1", "::1", "0.0.0.0", "100.100.100.100", "224.0.0.1", "bogus", "1.1.1.1", "1.1.1.1", "10.0.0.1"}, "wlan0")
	if strings.Join(got, ",") != "1.1.1.1,10.0.0.1" {
		t.Fatal(got)
	}
}

func TestSelectionPrefersWorkingNetworkThenFallback(t *testing.T) {
	probe := func(s string) error {
		if s == "10.0.0.1" {
			return os.ErrDeadlineExceeded
		}
		return nil
	}
	got, source, ok, _ := choose([]string{"10.0.0.1", "10.0.0.2"}, "android-linkproperties", []string{"1.1.1.1"}, probe)
	if source != "android-linkproperties" || !ok || strings.Join(got, ",") != "10.0.0.2" {
		t.Fatalf("%v %s %v", got, source, ok)
	}
	got, source, ok, _ = choose([]string{"10.0.0.1"}, "android-linkproperties", []string{"1.1.1.1"}, probe)
	if source != "public-fallback" || !ok || got[0] != "1.1.1.1" {
		t.Fatalf("%v %s %v", got, source, ok)
	}
}

func TestOfflineKeepsNewNetworkCandidatesNotOldNetwork(t *testing.T) {
	got, source, ok, _ := choose([]string{"10.2.0.1"}, "android-linkproperties", []string{"1.1.1.1"}, func(string) error { return os.ErrDeadlineExceeded })
	if ok || got[0] != "10.2.0.1" || source != "android-linkproperties-unverified" {
		t.Fatalf("%v %s %v", got, source, ok)
	}
}

func TestAtomicRefreshDoesNotChangeIdentity(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "tailscaled.state")
	os.WriteFile(state, []byte("identity-secret"), 0600)
	for _, value := range []string{"nameserver 10.0.0.1\n", "nameserver 10.2.0.1\n"} {
		if err := writeAtomic(filepath.Join(dir, "bootstrap-resolv.conf"), []byte(value)); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(dir, "bootstrap-resolv.conf"))
		if string(data) != value {
			t.Fatal(string(data))
		}
	}
	data, _ := os.ReadFile(state)
	info, _ := os.Stat(state)
	if string(data) != "identity-secret" || info.Mode().Perm() != 0600 {
		t.Fatal("state touched")
	}
}

func TestProbeActuallyResolvesAndRejectsRefusedDNS(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			end := 12
			for end < n && buf[end] != 0 {
				end += int(buf[end]) + 1
			}
			end += 5
			if end > n {
				continue
			}
			b := append([]byte(nil), buf[:end]...)
			b[2], b[3] = 0x81, 0x80
			binary.BigEndian.PutUint16(b[6:8], 1)
			binary.BigEndian.PutUint16(b[8:10], 0)
			binary.BigEndian.PutUint16(b[10:12], 0)
			// Echo question and return a valid A answer.
			b = append(b, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 192, 0, 2, 1)
			conn.WriteTo(b, addr)
		}
	}()
	if err := probeDNS(context.Background(), conn.LocalAddr().String(), "example.com", false); err != nil {
		t.Fatal(err)
	}
	closed, _ := net.ListenPacket("udp", "127.0.0.1:0")
	addr := closed.LocalAddr().String()
	closed.Close()
	if err := probeDNS(context.Background(), addr, "example.com", false); err == nil {
		t.Fatal("refused listener passed DNS check")
	}
}

func TestCLATAndNotVPNCapabilities(t *testing.T) {
	dump := `Active default network: 101
Current Networks:
 NetworkAgentInfo{network{101} ni{MOBILE CONNECTED} lp{{InterfaceName: rmnet_data0 DnsAddresses: [/10.0.0.1] Stacked: [{InterfaceName: v4-rmnet_data0 DnsAddresses: []}]}} nc{Capabilities: NOT_VPN}}
 NetworkAgentInfo{network{102} ni{VPN CONNECTED} lp{{InterfaceName: tun0 DnsAddresses: [/10.9.0.1]}}}`
	for _, iface := range []string{"", "rmnet_data0", "v4-rmnet_data0"} {
		got := connectivityDNS(dump, iface)
		if strings.Join(got, ",") != "10.0.0.1" {
			t.Fatalf("%q: %v", iface, got)
		}
	}
	if got := connectivityDNS(dump, "tun0"); strings.Join(got, ",") != "10.0.0.1" {
		t.Fatalf("VPN DNS leaked into bootstrap: %v", got)
	}
}
