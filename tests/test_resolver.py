#!/usr/bin/env python3
"""Real net.DefaultResolver must use and reload the build's bootstrap file.

With --baseline this must fail against the unmodified Go standard library.
The test overlay substitutes a temporary fixture path for the production path.
"""
import json
import os
import pathlib
import subprocess
import sys
import tempfile

def main():
    ROOT = pathlib.Path(__file__).resolve().parents[1]
    build = pathlib.Path(os.environ.get('BUILD_DIR', ROOT / 'build'))
    with tempfile.TemporaryDirectory() as tmp:
        tmp = pathlib.Path(tmp)
        bootstrap = tmp / 'bootstrap-resolv.conf'
        overlay = json.loads((build / 'overlay/overlay.json').read_text())
        replacements = {}
        for src, dst in overlay['Replace'].items():
            target = tmp / pathlib.Path(dst).name
            target.write_text(pathlib.Path(dst).read_text().replace('/data/adb/tailscale/bootstrap-resolv.conf', str(bootstrap)))
            replacements[src] = str(target)
        (tmp / 'overlay.json').write_text(json.dumps({'Replace': replacements}))
        test = r'''package main
    import (
     "context"
     "encoding/binary"
     "fmt"
     "net"
     "os"
     "sync"
     "time"
    )
    func main() {
     path := os.Args[1]
     conn, err := net.ListenPacket("udp", "127.0.0.1:0"); if err != nil { panic(err) }; defer conn.Close()
     go func() {
      buf := make([]byte, 1500)
      for { n, a, err := conn.ReadFrom(buf); if err != nil { return }
       end := 12; for end < n && buf[end] != 0 { end += int(buf[end])+1 }; end += 5; if end > n { continue }
       b := append([]byte(nil), buf[:end]...); b[2], b[3] = 0x81, 0x80
       binary.BigEndian.PutUint16(b[6:8], 1); binary.BigEndian.PutUint16(b[8:10], 0); binary.BigEndian.PutUint16(b[10:12], 0)
       b = append(b, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 192, 0, 2, 1); conn.WriteTo(b, a)
      }
     }()
     var mu sync.Mutex; var used []string
     net.DefaultResolver.PreferGo = true
     net.DefaultResolver.Dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
      mu.Lock(); used = append(used, addr); mu.Unlock()
      return (&net.Dialer{}).DialContext(ctx, network, conn.LocalAddr().String())
     }
     var priorMtime time.Time
     for i, server := range []string{"192.0.2.53", "198.51.100.53"} {
      // Separate writes across the kernel mtime tick and Go's 5s reload gate.
      if i > 0 { time.Sleep(6*time.Second) }
      temp := path+".tmp"; os.WriteFile(temp, []byte("nameserver "+server+"\noptions timeout:1 attempts:1\n"), 0600); os.Rename(temp, path)
      info, _ := os.Stat(path)
      if i > 0 && info.ModTime().Equal(priorMtime) { panic("fixture replacements share mtime") }
      priorMtime = info.ModTime()
      mu.Lock(); used = nil; mu.Unlock()
      ips, err := net.DefaultResolver.LookupIP(context.Background(), "ip4", "dns-regression.example"); if err != nil || len(ips) != 1 { panic(fmt.Sprintf("lookup failed: %v %v", ips, err)) }
      mu.Lock(); got := append([]string(nil), used...); mu.Unlock()
      if len(got) == 0 || got[0] != server+":53" { panic(fmt.Sprintf("Go read wrong resolver: got %v, want %s:53", got, server)) }
      fmt.Println("PASS bootstrap resolver", server, "without daemon restart")
     }
    }'''
        (tmp / 'main.go').write_text(test)
        command = ['go', 'run']
        if '--baseline' not in sys.argv:
            command += ['-overlay', str(tmp / 'overlay.json')]
        command += [str(tmp / 'main.go'), str(bootstrap)]
        env = {**os.environ, 'CGO_ENABLED': '0', 'GOOS': 'linux', 'GOARCH': 'amd64'}
        result = subprocess.run(command, cwd=tmp, env=env)
        sys.exit(result.returncode)

if __name__ == '__main__':
    main()
