package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type verifiedDNS struct {
	Network, Iface, Source, BootID string
	Servers                        []string
	Verified                       time.Time
}

// Empty fallback is a strict opt-out, including an already published public
// resolver. With no permitted Android candidates, publish no remote servers;
// startup remains pending and the running Go resolver drops the revoked list
// on its normal reload. Android verified caches are not affected.
func revokePublicBootstrap(dir string) error {
	var old verifiedDNS
	data, _ := os.ReadFile(filepath.Join(dir, "dns-verified.json"))
	_ = json.Unmarshal(data, &old)
	status, _ := os.ReadFile(filepath.Join(dir, "dns-status"))
	if old.Source != "public-fallback" && !strings.Contains(string(status), "dns_source=public-fallback") {
		return nil
	}
	if err := writeAtomic(filepath.Join(dir, "bootstrap-resolv.conf"), []byte("# Public DNS disabled; awaiting Android DNS\noptions timeout:1 attempts:1\n")); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(dir, "dns-status"), []byte("dns_source=waiting-for-android\ndns_servers=\ndns_reachable=false\ndns_retained=false\n")); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(dir, "dns-verified.json"))
	return nil
}

// Changing/clearing public fallback settings revokes cached public servers too.
func allowedCache(old verifiedDNS, fallback []string) verifiedDNS {
	if old.Source != "public-fallback" {
		return old
	}
	var allowed []string
	for _, server := range old.Servers {
		for _, candidate := range fallback {
			if candidate == server {
				allowed = append(allowed, server)
				break
			}
		}
	}
	old.Servers = allowed
	return old
}

func canRetain(old verifiedDNS, s Selection, now time.Time) bool {
	if len(old.Servers) == 0 || old.Verified.IsZero() || virtualInterface(old.Iface) {
		return false
	}
	if s.ID != "" {
		return s.ID == old.Network && s.Iface == old.Iface
	}
	if s.VPN != "" && s.Underlying != "default" && s.Underlying != "" {
		for _, id := range strings.Split(s.Underlying, ",") {
			if id == old.Network {
				return now.Sub(old.Verified) < 2*time.Minute
			}
		}
		return false // Known different underlying network invalidates old DNS.
	}
	return now.Sub(old.Verified) < 2*time.Minute
}

func readVerified(path string) verifiedDNS {
	var old verifiedDNS
	data, _ := os.ReadFile(path)
	_ = json.Unmarshal(data, &old)
	if old.BootID == "" || old.BootID != bootID() {
		return verifiedDNS{}
	}
	return old
}

func bootID() string {
	data, _ := os.ReadFile("/proc/sys/kernel/random/boot_id")
	return strings.TrimSpace(string(data))
}

func freshProbeContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 8*time.Second)
}
