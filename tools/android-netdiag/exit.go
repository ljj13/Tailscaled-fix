package main

import (
	"encoding/json"
	"sort"
	"strings"
)

// Public allowlist only. This is a preflight, not an exit-node setter or a claim
// that a selected exit node has successfully forwarded internet traffic.
func exitAuditSummary(prefs, status CommandResult) string {
	var b strings.Builder
	b.WriteString("mode=read-only\nrestoration_live_test=not-performed\n")
	var p struct {
		ExitNodeID, ExitNodeIP          *string
		ExitNodeAllowLANAccess, CorpDNS *bool
	}
	if prefs.Error != "" || prefs.Timeout || prefs.Truncated || json.Unmarshal([]byte(prefs.Stdout), &p) != nil {
		b.WriteString("configured=unknown\nprefs=<unavailable: unreadable or failed preferences>\n")
	} else {
		state := "unknown"
		if p.ExitNodeID != nil && p.ExitNodeIP != nil {
			state = "disabled"
		}
		if p.ExitNodeID != nil && *p.ExitNodeID != "" || p.ExitNodeIP != nil && *p.ExitNodeIP != "" {
			state = "enabled"
		}
		b.WriteString("configured=" + state + "\naccept_dns=" + boolText(p.CorpDNS) + "\nallow_lan_access=" + boolText(p.ExitNodeAllowLANAccess) + "\n")
	}
	type candidate struct {
		ID             string   `json:"id"`
		HostName       string   `json:"hostname"`
		TailscaleIPs   []string `json:"tailnet_ips"`
		Online         *bool    `json:"online"`
		ExitNodeOption *bool    `json:"exit_node_option"`
		ExitNode       *bool    `json:"selected"`
	}
	// Decode the upstream names separately from our explicit output tags.
	var input struct {
		Self struct{ ID string }
		Peer map[string]struct {
			ID, HostName                     string
			TailscaleIPs                     []string
			Online, ExitNodeOption, ExitNode *bool
		}
		ExitNodeStatus *struct {
			ID           string
			Online       *bool
			TailscaleIPs []string
		}
	}
	if status.Error != "" || status.Timeout || status.Truncated || json.Unmarshal([]byte(status.Stdout), &input) != nil {
		b.WriteString("status=<unavailable: unreadable or failed status JSON>\n")
	} else {
		candidates := []candidate{}
		for _, peer := range input.Peer {
			if peer.ID != "" && peer.ID == input.Self.ID {
				continue
			}
			if peer.ExitNodeOption != nil && *peer.ExitNodeOption || peer.ExitNode != nil && *peer.ExitNode {
				candidates = append(candidates, candidate{peer.ID, peer.HostName, peer.TailscaleIPs, peer.Online, peer.ExitNodeOption, peer.ExitNode})
			}
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].HostName != candidates[j].HostName {
				return candidates[i].HostName < candidates[j].HostName
			}
			return candidates[i].ID < candidates[j].ID
		})
		data, _ := json.MarshalIndent(candidates, "", "  ")
		b.WriteString("candidates=" + string(data) + "\n")
		if input.ExitNodeStatus != nil {
			data, _ = json.Marshal(input.ExitNodeStatus)
			b.WriteString("selected_status=" + string(data) + "\n")
		}
	}
	b.WriteString("internet_forwarding=not-tested\nDNS_scope=bootstrap / Tailscale resolver / Android-VPN resolver must be checked separately\n")
	return b.String()
}

func exitDefaultRoute(result CommandResult) string {
	if result.Error != "" || result.Timeout || result.Truncated {
		return "unknown (unavailable)"
	}
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] == "throw" {
			continue
		}
		for _, word := range fields[:min(2, len(fields))] {
			if word == "default" || word == "0.0.0.0/0" || word == "::/0" {
				return "present"
			}
		}
	}
	return "absent"
}
