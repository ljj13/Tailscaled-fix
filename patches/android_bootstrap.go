//go:build linux && !android

package dns

import (
	"bytes"
	"path/filepath"
	"sync"
	"time"
)

const androidBootstrapConf = "/data/adb/tailscale/bootstrap-resolv.conf"

// netlink can notify before the watchdog publishes Android DNS. Notify the
// existing DNS recompilation path so quad100 also refreshes its base forwarders.
func (m *directManager) runAndroidBootstrapWatcher() {
	watch, ok := HookWatchFile.GetOk()
	if m.trampleDNSPub == nil {
		return
	}
	previous, _ := m.fs.ReadFile(androidBootstrapConf)
	var mu sync.Mutex
	changed := func() {
		current, err := m.fs.ReadFile(androidBootstrapConf)
		if err != nil {
			return
		}
		mu.Lock()
		same := bytes.Equal(previous, current)
		previous = current
		mu.Unlock()
		if same {
			return
		}
		select {
		case <-m.ctx.Done():
			return
		default:
		}
		m.logf("dns: Android bootstrap DNS changed; recompiling base forwarders")
		m.trampleDNSPub.Publish(TrampleDNS{LastTrample: time.Now()})
	}
	path := m.fs.ActualPath(androidBootstrapConf)
	if ok {
		go func() {
			if err := watch(m.ctx, filepath.Dir(path), path, changed); err != nil && m.ctx.Err() == nil {
				m.logf("dns: Android bootstrap watcher: %v", err)
			}
		}()
	}
	// Recover missed registration/rename events and keep working if an OEM
	// cannot supply inotify. Only changed bytes publish a recompilation event.
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			changed()
		}
	}
}
