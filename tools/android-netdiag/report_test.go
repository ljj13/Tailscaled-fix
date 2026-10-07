package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReportUnifiedRedactionCanaries(t *testing.T) {
	secrets := []string{"AUTH_CANARY_001", "API_CANARY_002", "PRIVATE_CANARY_003", "NODE_CANARY_004", "MACHINE_CANARY_005", "COOKIE_CANARY_006", "OAUTH_CANARY_007", "CUSTOM_CANARY_008", "STATE_CANARY_009"}
	input := `{"AuthKey":"AUTH_CANARY_001","Persist":{"PrivateNodeKey":"PRIVATE_CANARY_003","NodeKey":"NODE_CANARY_004","MachineKey":"MACHINE_CANARY_005"},"tailscaled.state":"STATE_CANARY_009","hostname":"phone","IPv4":"100.72.239.86"}
Authorization: Bearer API_CANARY_002
Cookie: sid=COOKIE_CANARY_006
OAuthToken="OAUTH_CANARY_007"
custom_secret="CUSTOM_CANARY_008"
https://login.tailscale.com/a/AUTH_CANARY_001
https://example.com/oauth/callback?code=OAUTH_CANARY_007
https://example.com/service?api_token=API_CANARY_002
plain repeat CUSTOM_CANARY_008
nodekey:NODE_CANARY_004
-----BEGIN PRIVATE KEY-----
PRIVATE_CANARY_003
-----END PRIVATE KEY-----
interface=ccmni1 route=2001:db8::/64 DERP=hkg`
	out := redactReport(input)
	for _, secret := range secrets {
		if strings.Contains(out, secret) {
			t.Fatalf("leak %s in %s", secret, out)
		}
	}
	for _, required := range []string{"phone", "100.72.239.86", "ccmni1", "2001:db8::/64", "hkg"} {
		if !strings.Contains(out, required) {
			t.Fatal("lost network context", required)
		}
	}
	if strings.Contains(out, "https://login") || strings.Contains(out, "/oauth/") {
		t.Fatal("auth URL leak", out)
	}
}
func TestReportMalformedAndMultilineSecrets(t *testing.T) {
	for _, s := range []string{`{"PrivateKey":"BROKEN_CANARY",`, "auth_key=\nMULTILINE_CANARY\n", `tskey-auth-CANARY123`, "Authorization: Basic BASIC_CANARY\n", `login_url=https://evil.example/CANARY_URL`, "-----BEGIN PRIVATE KEY-----\nPEM_CANARY\n"} {
		out := redactReport(s)
		if strings.Contains(out, "CANARY") {
			t.Fatalf("leak in %q: %q", s, out)
		}
	}
}
func TestSafePrefsDropsPersistAndUnknownSecrets(t *testing.T) {
	out := safePreferences(CommandResult{Stdout: `{"Hostname":"phone","RouteAll":true,"CorpDNS":false,"AdvertiseRoutes":["192.168.1.0/24"],"Persist":{"PrivateKey":"CANARY"},"FutureSecret":"CANARY"}`})
	if strings.Contains(out.Stdout, "CANARY") || !strings.Contains(out.Stdout, "phone") {
		t.Fatal(out)
	}
	for _, bad := range []CommandResult{{Stdout: "CANARY invalid", Stderr: "CANARY error"}, {Stdout: `{"Hostname":"CANARY"}`, Truncated: true}, {Error: "failed", Stdout: "CANARY"}} {
		out = safePreferences(bad)
		if strings.Contains(out.Stdout, "CANARY") {
			t.Fatal(out)
		}
	}
}
func TestReportPartialFailureAndNoStateRead(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "run"), 0700)
	os.WriteFile(filepath.Join(dir, "run/tailscaled.state"), []byte("STATE_READ_CANARY"), 0600)
	os.WriteFile(filepath.Join(dir, "installed-module.prop"), []byte("version=v-test\n"), 0600)
	os.WriteFile(filepath.Join(dir, "run/tailscaled.log"), []byte("auth_key=LOG_CANARY\ninterface=ccmni1\n"), 0600)
	cli := filepath.Join(dir, "cli")
	os.WriteFile(cli, []byte("#!/bin/sh\nprintf 'token=CLI_CANARY\\n'\nexit 1\n"), 0700)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	out := diagnosticExport(ctx, Options{CLI: cli, Dir: dir, Mark: "0x10020000"})
	for _, secret := range []string{"STATE_READ_CANARY", "LOG_CANARY", "CLI_CANARY"} {
		if strings.Contains(out, secret) {
			t.Fatal("leak", secret, out)
		}
	}
	for _, required := range []string{"Tailscaled-fix Diagnostic Report\nversion: v-test\n", "redaction: enabled", "<unavailable:", "[DNS status]", "[table52 IPv4]", "[service log]"} {
		if !strings.Contains(out, required) {
			t.Fatal("missing", required, out)
		}
	}
}

func TestReportRefusesSymlinksAndArbitraryFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "run"), 0700)
	os.WriteFile(filepath.Join(dir, "run/tailscaled.state"), []byte("STATE_CANARY"), 0600)
	os.Symlink(filepath.Join(dir, "run/tailscaled.state"), filepath.Join(dir, "run/diag.log"))
	if publicReportPath(dir, "diag.log", true) == nil {
		t.Fatal("symlink accepted")
	}
	if publicReportPath(dir, "tailscaled.state", false) == nil {
		t.Fatal("state accepted")
	}
	if publicReportPath(dir, "settings.ini", false) == nil {
		t.Fatal("settings accepted")
	}
}
func TestReportKeepsPublicNetworkStatus(t *testing.T) {
	out := redactReport("outer_ipv6_state=active\nBackendState=Running\ndns_iface=ccmni1\n")
	for _, v := range []string{"outer_ipv6_state=active", "BackendState=Running", "dns_iface=ccmni1"} {
		if !strings.Contains(out, v) {
			t.Fatal(out)
		}
	}
}

func TestReportDropsWholeStateDump(t *testing.T) {
	out := redactReport(`state dump {"_machinekey":"mkey:STATE_KEY_CANARY","_profiles":{"arbitrary":"STATE_BODY_CANARY"}}`)
	if strings.Contains(out, "CANARY") {
		t.Fatal("state dump leaked", out)
	}
}
