// android-hostname initializes only the hostname preference. It never opens
// tailscaled.state, touches routing, or changes the daemon's reported OS.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
)

var separators = regexp.MustCompile(`[^a-z0-9]+`)

func normalize(raw string) string {
	if strings.ContainsAny(raw, "\r\n") {
		return ""
	}
	name := strings.Trim(separators.ReplaceAllString(strings.ToLower(raw), "-"), "-")
	if len(name) > 63 {
		name = strings.TrimRight(name[:63], "-")
	}
	switch name {
	case "null", "unknown", "localhost", "localhost-0":
		return ""
	}
	return name
}

func command(timeout time.Duration, binary string, args ...string) ([]byte, error) {
	// Pdeathsig is tied to the creating OS thread on Linux. Keep that thread
	// alive until Wait finishes; kill a pending CLI write if the helper crashes.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	cmd.WaitDelay = 500 * time.Millisecond
	return cmd.Output()
}

func hostname(cli string, args []string) (string, error) {
	data, err := command(3*time.Second, cli, append(append([]string{}, args...), "debug", "prefs")...)
	if err != nil {
		return "", err
	}
	var prefs map[string]json.RawMessage
	if err := json.Unmarshal(data, &prefs); err != nil {
		return "", err
	}
	var value *string
	if err := json.Unmarshal(prefs["Hostname"], &value); err != nil || value == nil {
		return "", errors.New("preferences have no Hostname string")
	}
	return *value, nil
}

type candidate struct {
	source string
	cmd    string
	args   []string
}

func deviceName() (string, string, error) {
	candidates := []candidate{
		{"settings.global.device_name", "settings", []string{"get", "global", "device_name"}},
		{"prop.persist.sys.device_name", "getprop", []string{"persist.sys.device_name"}},
		{"settings.secure.bluetooth_name", "settings", []string{"get", "secure", "bluetooth_name"}},
	}
	for _, prop := range []string{"ro.product.marketname", "ro.product.vendor.marketname", "ro.product.odm.marketname",
		"ro.product.model", "ro.product.vendor.model", "ro.product.device", "ro.product.vendor.device"} {
		candidates = append(candidates, candidate{"prop." + prop, "getprop", []string{prop}})
	}
	for _, c := range candidates {
		data, err := command(time.Second, c.cmd, c.args...)
		if err != nil {
			continue
		}
		// Remove the utility's terminating newline, retaining internal newlines.
		name := normalize(strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r"))
		if name != "" {
			return c.source, name, nil
		}
	}
	return "", "", errors.New("Android device name unavailable; will retry")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	// Unreadable markers also fail closed.
	return !os.IsNotExist(err)
}

func lock(dir string, wait bool) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, "hostname-init.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(35 * time.Second)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			// The lock fd is close-on-exec: child CLI/settings processes cannot
			// keep it locked after a helper crash. Never unlink this inode.
			return f, nil
		}
		if !wait || (!errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN)) || time.Now().After(deadline) {
			f.Close()
			return nil, err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func record(dir, source, name string) error {
	f, err := os.CreateTemp(dir, "hostname-initialized.tmp.")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = fmt.Fprintf(f, "source=%s\nhostname=%s\n", source, name)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(dir, "hostname-initialized"))
}

func run(action, dir, cli string, args []string) error {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) == "/" {
		return errors.New("expected an absolute installation directory")
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return errors.New("installation directory unavailable")
	}
	done := filepath.Join(dir, "hostname-initialized")
	manual := filepath.Join(dir, "hostname-user-set")
	if action == "protect" {
		// Explicit intent opts out permanently, even if the following CLI
		// command fails. Wait for any in-flight write before allowing the
		// user's command to execute, so the manual value always wins.
		if err := os.MkdirAll(manual, 0700); err != nil {
			return err
		}
		f, err := lock(dir, true)
		if err != nil {
			return err
		}
		return f.Close()
	}
	if action != "init" || cli == "" {
		return errors.New("usage: android-hostname init DIR CLI [global arguments...] | protect DIR")
	}
	if exists(done) || exists(manual) {
		return nil
	}
	f, err := lock(dir, false)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return nil // Another initializer owns this attempt.
	}
	if err != nil {
		return err
	}
	defer f.Close()
	if exists(done) || exists(manual) {
		return nil
	}
	current, err := hostname(cli, args)
	if err != nil {
		return err
	}
	if current != "" {
		return record(dir, "preserved-explicit", "")
	}
	source, name, err := deviceName()
	if err != nil {
		return err
	}
	// Also protect a direct CLI change made during Android discovery. All
	// module wrapper/service hostname writes use the permanent intent marker.
	current, err = hostname(cli, args)
	if err != nil {
		return err
	}
	if exists(manual) {
		return nil
	}
	if current != "" {
		return record(dir, "preserved-explicit", "")
	}
	if _, err := command(3*time.Second, cli, append(append([]string{}, args...), "set", "--hostname="+name)...); err != nil {
		return err
	}
	if err := record(dir, source, name); err != nil {
		return err
	}
	fmt.Printf("hostname-init: source=%s hostname=%s (saved once)\n", source, name)
	return nil
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "hostname-init: missing action/directory")
		os.Exit(1)
	}
	cli := ""
	var args []string
	if len(os.Args) >= 4 {
		cli, args = os.Args[3], os.Args[4:]
	}
	if err := run(os.Args[1], os.Args[2], cli, args); err != nil {
		fmt.Fprintln(os.Stderr, "hostname-init: pending:", err)
		os.Exit(1)
	}
}
