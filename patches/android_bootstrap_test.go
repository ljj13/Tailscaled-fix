//go:build linux && !android

package dns

import (
	"context"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"tailscale.com/util/eventbus/eventbustest"
)

func TestAndroidBootstrapRefresh(t *testing.T) {
	fs := directFS{prefix: t.TempDir()}
	os.MkdirAll(filepath.Dir(fs.ActualPath(androidBootstrapConf)), 0700)
	fs.WriteFile(androidBootstrapConf, []byte("nameserver 10.0.0.1\n"), 0600)
	watchReady := make(chan func(), 1)
	osReady := make(chan struct{})
	osDone := make(chan struct{})
	bootstrapDone := make(chan struct{})
	t.Cleanup(HookWatchFile.SetForTest(func(ctx context.Context, _, file string, cb func()) error {
		if file == fs.ActualPath(androidBootstrapConf) {
			watchReady <- cb
		} else {
			close(osReady)
		}
		<-ctx.Done()
		if file == fs.ActualPath(androidBootstrapConf) {
			close(bootstrapDone)
		} else {
			close(osDone)
		}
		return ctx.Err()
	}))
	bus := eventbustest.NewBus(t)
	m := newDirectManagerOnFS(t.Logf, nil, bus, fs)
	defer func() { m.Close(); <-osDone; <-bootstrapDone }()
	var cb func()
	select {
	case cb = <-watchReady:
	case <-time.After(3 * time.Second):
		t.Fatal("bootstrap watcher not registered")
	}
	<-osReady
	if err := m.SetDNS(OSConfig{Nameservers: []netip.Addr{netip.MustParseAddr("100.100.100.100")}}); err != nil {
		t.Fatal(err)
	}
	before, err := m.GetBaseConfig()
	if err != nil || before.Nameservers[0].String() != "10.0.0.1" {
		t.Fatalf("before: %v %v", before, err)
	}
	watcher := eventbustest.NewWatcher(t, bus)
	fs.WriteFile(androidBootstrapConf, []byte("nameserver 10.2.0.1\n"), 0600)
	cb()
	if err := eventbustest.Expect(watcher, eventbustest.Type[TrampleDNS]()); err != nil {
		t.Fatal(err)
	}
	after, err := m.GetBaseConfig()
	if err != nil || after.Nameservers[0].String() != "10.2.0.1" {
		t.Fatalf("after: %v %v", after, err)
	}
	data, _ := fs.ReadFile(resolvConf)
	if len(data) == 0 {
		t.Fatal("accept-dns config was removed")
	}
}

func TestAndroidBootstrapMissedRegistrationEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fs := directFS{prefix: t.TempDir()}
		os.MkdirAll(filepath.Dir(fs.ActualPath(androidBootstrapConf)), 0700)
		fs.WriteFile(androidBootstrapConf, []byte("nameserver 10.0.0.1\n"), 0600)
		ready := make(chan struct{})
		t.Cleanup(HookWatchFile.SetForTest(func(ctx context.Context, _, file string, _ func()) error {
			if file == fs.ActualPath(androidBootstrapConf) {
				// Replacement between baseline read and watch registration: no callback.
				fs.WriteFile(androidBootstrapConf, []byte("nameserver 10.2.0.1\n"), 0600)
				close(ready)
			}
			<-ctx.Done()
			return ctx.Err()
		}))
		bus := eventbustest.NewBus(t)
		watcher := eventbustest.NewWatcher(t, bus)
		m := newDirectManagerOnFS(t.Logf, nil, bus, fs)
		defer func() {
			m.Close()
			// Drain watcher startup/shutdown before t.Cleanup restores the hook.
			// Otherwise a late GetOk can race the SetForTest cleanup write.
			synctest.Wait()
		}()
		<-ready
		if err := eventbustest.Expect(watcher, eventbustest.Type[TrampleDNS]()); err != nil {
			t.Fatal(err)
		}
	})
}
