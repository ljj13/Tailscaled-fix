package main

import (
	"strings"
	"testing"
)

func TestExitAuditUnknownDisabledAndSelected(t *testing.T) {
	for _, tc := range []struct{ prefs, status, want string }{
		{`{}`, `{}`, "configured=unknown"},
		{`{"ExitNodeID":"","ExitNodeIP":""}`, `{}`, "configured=disabled"},
		{`{"ExitNodeID":"n1","ExitNodeIP":"","ExitNodeAllowLANAccess":false,"CorpDNS":false}`, `{"ExitNodeStatus":{"ID":"n1","Online":false,"TailscaleIPs":["100.72.239.86"]}}`, "configured=enabled"},
	} {
		out := exitAuditSummary(CommandResult{Stdout: tc.prefs}, CommandResult{Stdout: tc.status})
		if !strings.Contains(out, tc.want) || !strings.Contains(out, "restoration_live_test=not-performed") {
			t.Fatal(out)
		}
	}
	out := exitAuditSummary(CommandResult{Error: "permission denied", Stdout: `{"ExitNodeID":""}`}, CommandResult{Stdout: `{broken`})
	if !strings.Contains(out, "unavailable:") || strings.Contains(out, "configured=disabled") {
		t.Fatal(out)
	}
}

func TestExitAuditCandidatesOfflineAndNoSecretProjection(t *testing.T) {
	prefs := safePreferences(CommandResult{Stdout: `{"ExitNodeID":"n1","ExitNodeAllowLANAccess":true,"CorpDNS":true,"Persist":{"PrivateKey":"EXIT_KEY_CANARY"}}`})
	status := CommandResult{Stdout: `{"Self":{"ID":"self"},"Peer":{"x":{"ID":"n1","HostName":"exit","Online":false,"ExitNodeOption":true,"ExitNode":true,"TailscaleIPs":["100.72.239.86"],"PrivateKey":"EXIT_KEY_CANARY"},"y":{"ID":"self","ExitNodeOption":true},"z":{"HostName":"other","Online":true}}}`}
	out := exitAuditSummary(prefs, status)
	if strings.Contains(out, "CANARY") || strings.Contains(out, "self") || strings.Contains(out, "other") || !strings.Contains(out, `"online": false`) || !strings.Contains(out, "accept_dns=true") {
		t.Fatal(out)
	}
	if !strings.Contains(prefs.Stdout, "ExitNodeID") || strings.Contains(prefs.Stdout, "CANARY") {
		t.Fatal(prefs)
	}
}

func TestExitAuditDefaultRouteResidualRisk(t *testing.T) {
	for _, tc := range []struct{ route, want string }{
		{"100.64.0.0/10 dev tailscale0", "absent"},
		{"default dev tailscale0", "present"},
		{"::/0 dev tailscale0", "present"},
		{"blackhole default", "present"},
		{"throw default", "absent"},
	} {
		if got := exitDefaultRoute(CommandResult{Stdout: tc.route}); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	if !strings.HasPrefix(exitDefaultRoute(CommandResult{Error: "permission denied"}), "unknown") {
		t.Fatal("failed route query claimed absent")
	}
}
