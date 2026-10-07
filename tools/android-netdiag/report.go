package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const redacted = "[REDACTED]"

// Single deny policy for every section (including stderr/errors). New collectors
// cannot write directly to stdout: only diagnosticExport's final redactor may emit.
var secretField = regexp.MustCompile(`(?i)["']?([a-z0-9_.-]*(?:auth|oauth|token|secret|password|credential|cookie|private|nodekey|machinekey|publickey|login|persist|state|socket)[a-z0-9_.-]*|[a-z0-9_.-]*key)["']?\s*[:=]\s*`)
var secretURL = regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)
var keyToken = regexp.MustCompile(`(?i)(?:tskey-[a-z]+-|(?:private|privkey|nodekey|machinekey|mkey):)[a-z0-9_+./=-]+`)
var pemBlock = regexp.MustCompile(`(?s)-----BEGIN [^-]*PRIVATE KEY-----.*?(?:-----END [^-]*PRIVATE KEY-----|$)`)
var authHeader = regexp.MustCompile(`(?im)(?:Authorization|Proxy-Authorization|Cookie|Set-Cookie)\s*:\s*[^\r\n]*`)

func sensitiveField(key string) bool {
	if key == "outer_ipv6_state" || key == "BackendState" {
		return false
	}
	return secretField.MatchString(key + "=")
}
func rememberSecrets(value any, secrets *[]string) {
	switch v := value.(type) {
	case string:
		if v != "" && v != redacted {
			*secrets = append(*secrets, v)
			for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == '=' || r == ';' }) {
				if len(part) >= 4 && part != "Bearer" && part != "Basic" {
					*secrets = append(*secrets, part)
				}
			}
		}
	case map[string]any:
		for _, child := range v {
			rememberSecrets(child, secrets)
		}
	case []any:
		for _, child := range v {
			rememberSecrets(child, secrets)
		}
	}
}
func scrubObject(value any, secrets *[]string) any {
	switch v := value.(type) {
	case map[string]any:
		for _, key := range []string{"_machinekey", "_profiles", "_current-profile"} {
			if _, ok := v[key]; ok {
				rememberSecrets(v, secrets)
				return redacted
			}
		}
		for key, child := range v {
			if sensitiveField(key) {
				rememberSecrets(child, secrets)
				v[key] = redacted
			} else {
				v[key] = scrubObject(child, secrets)
			}
		}
	case []any:
		for i, child := range v {
			v[i] = scrubObject(child, secrets)
		}
	}
	return value
}
func redactReport(input string) string {
	secrets := []string{}
	// Decode complete JSON objects even when embedded in timestamped log lines.
	var out strings.Builder
	for len(input) > 0 {
		at := strings.IndexByte(input, '{')
		if at < 0 {
			out.WriteString(input)
			break
		}
		out.WriteString(input[:at])
		input = input[at:]
		decoder := json.NewDecoder(strings.NewReader(input))
		decoder.UseNumber()
		var value any
		if decoder.Decode(&value) == nil {
			safe := scrubObject(value, &secrets)
			data, _ := json.Marshal(safe)
			out.Write(data)
			input = input[decoder.InputOffset():]
		} else {
			out.WriteByte('{')
			input = input[1:]
		}
	}
	text := out.String()
	text = pemBlock.ReplaceAllStringFunc(text, func(block string) string { rememberSecrets(block, &secrets); return redacted })
	// Fail closed on malformed JSON, shell-style fields and multiline assignments.
	matches := secretField.FindAllStringSubmatchIndex(text, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		start, end := matches[i][0], matches[i][1]
		if !sensitiveField(text[matches[i][2]:matches[i][3]]) {
			continue
		}
		tail := text[end:]
		length := strings.IndexAny(tail, "\r\n")
		if length < 0 {
			length = len(tail)
		}
		if length == 0 && len(tail) > 0 {
			tail = tail[1:]
			length = strings.IndexAny(tail, "\r\n")
			if length < 0 {
				length = len(tail)
			}
			length++
		}
		value := text[end : end+length]
		if strings.HasPrefix(value, `"`) {
			for j := 1; j < len(value); j++ {
				if value[j] == '"' && value[j-1] != '\\' {
					if decoded, err := strconv.Unquote(value[:j+1]); err == nil {
						value = decoded
						length = j + 1
					}
					break
				}
			}
		}
		rememberSecrets(strings.Trim(strings.TrimSpace(value), "\"',"), &secrets)
		text = text[:start] + redacted + text[end+length:]
	}
	text = authHeader.ReplaceAllStringFunc(text, func(v string) string { rememberSecrets(v, &secrets); return redacted })
	text = secretURL.ReplaceAllStringFunc(text, func(url string) string {
		lower := strings.ToLower(url)
		for _, part := range []string{"login.", "/login", "/auth", "/oauth", "/a/", "token=", "key=", "code=", "secret=", "password="} {
			if strings.Contains(lower, part) {
				rememberSecrets(url, &secrets)
				return redacted
			}
		}
		return url
	})
	text = keyToken.ReplaceAllStringFunc(text, func(v string) string { rememberSecrets(v, &secrets); return redacted })
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		if secret != "" && secret != redacted {
			text = strings.ReplaceAll(text, secret, redacted)
		}
	}
	return text
}

func safePreferences(result CommandResult) CommandResult {
	// An allowlist, not a blacklist: Persist and future prefs never enter a report.
	var prefs struct {
		Hostname                                  string
		ExitNodeID, ExitNodeIP                    *string
		ExitNodeAllowLANAccess                    *bool
		RouteAll, CorpDNS, ShieldsUp, WantRunning *bool
		AdvertiseRoutes                           []string
	}
	if result.Error != "" || result.Timeout || result.Truncated {
		result.Stdout = ""
		result.Stderr = ""
		return result
	}
	if json.Unmarshal([]byte(result.Stdout), &prefs) != nil {
		result.Stdout = ""
		result.Stderr = ""
		result.Error = "unreadable preferences JSON"
		return result
	}
	data, _ := json.Marshal(prefs)
	result.Stdout = string(data)
	result.Stderr = ""
	return result
}
func reportResult(value CommandResult) string {
	if value.Timeout {
		return "<unavailable: timeout>"
	}
	if value.Error != "" {
		return "<unavailable: " + value.Error + ">\n" + value.Stderr
	}
	if value.Truncated {
		return "<unavailable: output limit exceeded>"
	}
	text := strings.TrimSpace(value.Stdout + "\n" + value.Stderr)
	if text == "" {
		return "<unavailable: no data>"
	}
	return text
}

// Only these fixed public metadata/status files may be read. Never read state,
// settings.ini, sockets, arbitrary user paths or symlinks to sensitive files.
func publicReportPath(dir, name string, log bool) error {
	switch name {
	case "installed-module.prop", "build-info.json", "dns-status", "outer-ipv6-status", "service.log", "diag.log", "tailscaled.log":
	default:
		return fmt.Errorf("not a permitted report file")
	}
	paths := []string{dir}
	if log {
		paths = append(paths, filepath.Join(dir, "run"))
	}
	paths = append(paths, filepath.Join(paths[len(paths)-1], name))
	for i, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink report path refused")
		}
		if i == len(paths)-1 && !info.Mode().IsRegular() {
			return fmt.Errorf("not a regular report file")
		}
	}
	return nil
}
func reportFile(dir, name string) CommandResult {
	if err := publicReportPath(dir, name, false); err != nil {
		return CommandResult{Error: err.Error()}
	}
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if err != nil {
		return CommandResult{Error: err.Error()}
	}
	if !info.Mode().IsRegular() || info.Size() > outputLimit {
		return CommandResult{Error: "not a bounded regular file"}
	}
	f, err := os.Open(path)
	if err != nil {
		return CommandResult{Error: err.Error()}
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, outputLimit+1))
	if err != nil {
		return CommandResult{Error: err.Error()}
	}
	if len(data) > outputLimit {
		return CommandResult{Error: "output limit exceeded"}
	}
	return CommandResult{Stdout: string(data)}
}
func diagnosticExport(ctx context.Context, o Options) string {
	o.Export = true
	jobs := map[string][]string{
		"tailscale status":                    append([]string{o.CLI}, append(append([]string{}, o.Args...), "status")...),
		"prefs safe fields":                   append([]string{o.CLI}, append(append([]string{}, o.Args...), "debug", "prefs")...),
		"daemon/backend and proxy exemptions": {filepath.Join(o.Dir, "scripts/tailscaled.service"), "webstatus"},
		"table52 IPv4":                        {"ip", "-4", "route", "show", "table", "52"}, "table52 IPv6": {"ip", "-6", "route", "show", "table", "52"},
		"table1099 IPv4": {"ip", "-4", "route", "show", "table", "1099"}, "table1099 IPv6": {"ip", "-6", "route", "show", "table", "1099"},
	}
	if o.ExitAudit {
		jobs["unmarked IPv4 route"] = []string{"ip", "-4", "route", "get", "1.1.1.1"}
		jobs["unmarked IPv6 route"] = []string{"ip", "-6", "route", "get", "2606:4700:4700::1111"}
		for _, table := range []string{"mangle", "nat"} {
			jobs[table+" OUTPUT rules"] = []string{"iptables", "-t", table, "-S", "OUTPUT"}
		}
	}
	// Fixed logs only, bounded tail; the redactor runs after all results are joined.
	for _, name := range []string{"service", "diag", "tailscaled"} {
		jobs[name+" log"] = []string{"tail", "-n", "120", filepath.Join(o.Dir, "run", name+".log")}
	}
	values := map[string]CommandResult{}
	var mutex sync.Mutex
	var wg sync.WaitGroup
	for key, command := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value := CommandResult{}
			if strings.HasSuffix(key, " log") {
				if err := publicReportPath(o.Dir, filepath.Base(command[len(command)-1]), true); err != nil {
					value.Error = err.Error()
				} else {
					value = runCommand(ctx, command[0], command[1:]...)
				}
			} else {
				value = runCommand(ctx, command[0], command[1:]...)
			}
			if key == "prefs safe fields" {
				value = safePreferences(value)
			}
			mutex.Lock()
			values[key] = value
			mutex.Unlock()
		}()
	}
	r := collect(ctx, o)
	wg.Wait()
	if o.ExitAudit {
		values["Exit Node Client preflight"] = CommandResult{Stdout: exitAuditSummary(values["prefs safe fields"], r.Raw["status"])}
		values["Exit Node restoration risk"] = CommandResult{Stdout: "table1099_ipv4_default=" + exitDefaultRoute(values["table1099 IPv4"]) + "\ntable1099_ipv6_default=" + exitDefaultRoute(values["table1099 IPv6"]) + "\nDefaults in module-owned/manual routes are not removed by tailscale set --exit-node=; never auto-delete foreign routing state.\n"}
	}
	// status JSON is summarized into the existing typed public model, never dumped.
	values["status JSON safe summary"] = CommandResult{Stdout: "backend=" + r.Backend + "\nversion=" + r.Version}
	summary := struct {
		Self         Node
		Peers        []Node
		Endpoints    []Endpoint
		Netcheck     Netcheck
		Network      map[string]string
		UDPListeners []Endpoint
	}{r.Self, r.Peers, r.Endpoints, r.Netcheck, r.Network, r.UDPListeners}
	if status := r.Raw["status"]; status.Error != "" || status.Timeout || status.Truncated || !json.Valid([]byte(status.Stdout)) {
		values["status JSON safe summary"] = CommandResult{Error: "status JSON unavailable: " + status.Error, Timeout: status.Timeout, Truncated: status.Truncated}
	} else {
		data, _ := json.MarshalIndent(summary, "", "  ")
		values["status JSON safe summary"] = CommandResult{Stdout: string(data)}
	}
	delete(r.Raw, "status")
	delete(r.Raw, "preferences")
	delete(r.Raw, "magicsock_log")
	for key, value := range r.Raw {
		values[key] = value
	}
	values["DNS status"] = reportFile(o.Dir, "dns-status")
	values["outer IPv6 status"] = reportFile(o.Dir, "outer-ipv6-status")
	prop := reportFile(o.Dir, "installed-module.prop")
	values["module metadata"] = prop
	values["build metadata"] = reportFile(o.Dir, "build-info.json")
	version := ""
	for _, line := range strings.Split(prop.Stdout, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok && key == "version" {
			version = strings.TrimSpace(value)
			break
		}
	}
	if version == "" {
		version = "unknown"
	}
	build := "unknown"
	var buildFields struct {
		ModuleRevision  string `json:"module_revision"`
		TailscaleCommit string `json:"tailscale_commit"`
	}
	if json.Unmarshal([]byte(values["build metadata"].Stdout), &buildFields) == nil {
		build = buildFields.ModuleRevision + " / tailscale " + buildFields.TailscaleCommit
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Tailscaled-fix Diagnostic Report\nversion: %s\nbuild: %s\ngenerated: %s\nredaction: enabled\n", version, build, time.Now().UTC().Format(time.RFC3339))
	keys := []string{}
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&b, "\n[%s]\n%s\n", key, reportResult(values[key]))
	}
	for _, err := range r.Errors {
		fmt.Fprintf(&b, "\n[collection error]\n<unavailable: %s>\n", err)
	}
	return redactReport(b.String())
}
