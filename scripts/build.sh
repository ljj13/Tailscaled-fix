#!/bin/sh
# Reproducible arm64 linux build. Run under Linux/WSL with Go 1.26.6 and Python3.
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
BUILD_DIR=${BUILD_DIR:-"$ROOT/build"}
TS_SOURCE=${TS_SOURCE:-"$BUILD_DIR/tailscale"}
mkdir -p "$BUILD_DIR" "$ROOT/files" "$ROOT/dist"
if [ ! -d "$TS_SOURCE/.git" ]; then
  git clone --depth 1 --branch v1.102.5 https://github.com/tailscale/tailscale.git "$TS_SOURCE"
fi
[ "$(git -C "$TS_SOURCE" rev-parse HEAD)" = "5fb2a81b065b0a0bbbfc67ab20a0d9c6a1108115" ] || {
  echo 'Refusing to build a different Tailscale revision' >&2; exit 1;
}
[ -z "$(git -C "$TS_SOURCE" status --porcelain)" ] || {
  echo 'Use a clean Tailscale source checkout (prepare-build modifies it)' >&2; exit 1;
}
[ "$(go env GOVERSION)" = "go1.26.6" ] || {
  echo 'This build requires Go 1.26.6; no silent toolchain drift' >&2; exit 1;
}
python3 "$ROOT/scripts/prepare-build.py" "$TS_SOURCE" "$BUILD_DIR/overlay"
python3 "$ROOT/scripts/prepare-tests.py" "$TS_SOURCE" "$BUILD_DIR/test-overlay"
OVERLAY="$BUILD_DIR/overlay/overlay.json"
(
  cd "$TS_SOURCE"
  go test -overlay "$BUILD_DIR/test-overlay/overlay.json" ./net/dns ./net/dnscache ./net/netns ./wgengine/router/osrouter
  go test -race -run TestAndroidBootstrap ./net/dns
  eval "$(CGO_ENABLED=0 go run ./cmd/mkversion)"
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -overlay "$OVERLAY" -trimpath \
    -tags ts_include_cli,ts_omit_ssh \
    -ldflags "-X tailscale.com/version.longStamp=${VERSION_LONG}-android-dnsfix.2 -X tailscale.com/version.shortStamp=${VERSION_SHORT} -s -w" \
    -o "$ROOT/files/tailscale.combined" ./cmd/tailscaled
)
(
  cd "$ROOT/tools/android-dns"
  go test -race ./...
  go vet ./...
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags '-s -w' -o "$ROOT/files/android-dns" .
)
python3 "$ROOT/scripts/build-hostname.py"
python3 "$ROOT/scripts/build-netdiag.py"
python3 "$ROOT/scripts/package.py"
