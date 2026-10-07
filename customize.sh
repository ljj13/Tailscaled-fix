#!/system/bin/sh
SKIPUNZIP=0
SKIPMOUNT=false

if [ "$BOOTMODE" != true ]; then
  ui_print "! Please install in Magisk / KernelSU / APatch Manager"
  ui_print "! Install from recovery is NOT supported"
  abort "-----------------------------------------------------------"
elif [ "$KSU" = true ] && [ "$KSU_VER_CODE" -lt 10670 ]; then
  abort "error: Please update your KernelSU and KernelSU Manager"
fi

if [ "$ARCH" != "arm64" ]; then
  abort "! Unsupported architecture: $ARCH (arm64 only)"
fi

SERVICE_DIR="/data/adb/service.d"
INSTALL_DIR="/data/adb/tailscale"
INSTALL_BIN_DIR="$INSTALL_DIR/bin"

# Refuse incomplete payloads before touching the running installation.
for f in tailscale.combined android-dns android-hostname android-netdiag; do
  [ -s "$MODPATH/files/$f" ] || abort "! Missing required binary: $f"
done

# Rollback assistance only: never snapshot state/keys, sockets, logs or binaries.
# Complete this before stopping the daemon or overwriting any installed script.
backup_upgrade() (
  for _ub_check in settings.ini routes scripts hostname-initialized hostname-user-set installed-module.prop; do
    [ ! -L "$INSTALL_DIR/$_ub_check" ] || exit 1
  done
  [ -e "$INSTALL_DIR/settings.ini" ] || [ -e "$INSTALL_DIR/routes" ] || [ -d "$INSTALL_DIR/scripts" ] || exit 0
  umask 077
  _ub_root="$INSTALL_DIR/backups"
  [ ! -L "$INSTALL_DIR" ] && [ ! -L "$_ub_root" ] || exit 1
  mkdir -p "$_ub_root" || exit 1
  # No redirects through symlinks, including old metadata and lock files.
  _ub_special=$(find "$_ub_root" ! -type f ! -type d -print) || exit 1
  [ -z "$_ub_special" ] || exit 1
  chmod 0700 "$_ub_root" || exit 1
  chown 0:0 "$_ub_root" || exit 1
  exec 9>"$_ub_root/.lock" || exit 1
  command -v flock >/dev/null 2>&1 && flock -n 9 9<&9 || exit 1
  chmod 0600 "$_ub_root/.lock" || exit 1
  _ub_old=unknown
  if [ -f "$INSTALL_DIR/installed-module.prop" ]; then
    [ ! -L "$INSTALL_DIR/installed-module.prop" ] || exit 1
    _ub_old=$(sed -n 's/^version=//p' "$INSTALL_DIR/installed-module.prop" | tr -d '\r')
  fi
  _ub_new=$(sed -n 's/^version=//p' "$MODPATH/module.prop" 2>/dev/null | tr -d '\r')
  _ub_new=${_ub_new:-unknown}
  for _ub_v in "$_ub_old" "$_ub_new"; do
    case "$_ub_v" in ''|.|..|*[!a-zA-Z0-9._-]*) exit 1 ;; esac
  done
  _ub_seq=$(cat "$_ub_root/.sequence" 2>/dev/null)
  _ub_seq=${_ub_seq:-0}
  case "$_ub_seq" in *[!0-9]*|?????????*) exit 1 ;; esac
  _ub_seq=$((_ub_seq + 1))
  printf '%s\n' "$_ub_seq" > "$_ub_root/.sequence" || exit 1
  chmod 0600 "$_ub_root/.sequence" || exit 1
  mkdir -p "$_ub_root/$_ub_old" || exit 1
  chmod 0700 "$_ub_root/$_ub_old" || exit 1
  # A crashed writer's staging directories are safe to remove under this lock.
  for _ub_pending in "$_ub_root"/*/.pending-*; do
    [ -d "$_ub_pending" ] || continue
    _ub_pid=${_ub_pending##*/.pending-}
    case "$_ub_pid" in ''|*[!0-9]*) continue ;; esac
    rm -rf "$_ub_pending" || exit 1
  done
  _ub_tmp="$_ub_root/$_ub_old/.pending-$$"
  _ub_dest="$_ub_root/$_ub_old/$(printf '%08d' "$_ub_seq")"
  [ ! -e "$_ub_dest" ] || exit 1
  mkdir "$_ub_tmp" || exit 1
  trap 'rm -rf "$_ub_tmp"' 0
  trap 'exit 1' 1 2 15
  for _ub_name in settings.ini routes hostname-initialized hostname-user-set scripts installed-module.prop build-info.json; do
    _ub_src="$INSTALL_DIR/$_ub_name"
    [ ! -L "$_ub_src" ] || exit 1
    [ -e "$_ub_src" ] || continue
    _ub_special=$(find "$_ub_src" ! -type f ! -type d -print) || exit 1
    [ -z "$_ub_special" ] || exit 1
    cp -R "$_ub_src" "$_ub_tmp/$_ub_name" || exit 1
  done
  if [ -e "$SERVICE_DIR/tailscaled_service.sh" ] || [ -L "$SERVICE_DIR/tailscaled_service.sh" ]; then
    [ ! -L "$SERVICE_DIR" ] && [ ! -L "$SERVICE_DIR/tailscaled_service.sh" ] && [ -f "$SERVICE_DIR/tailscaled_service.sh" ] || exit 1
    cp "$SERVICE_DIR/tailscaled_service.sh" "$_ub_tmp/boot-service.sh" || exit 1
  fi
  printf 'format=tailscaled-upgrade-backup-v1\nsource_version=%s\ntarget_version=%s\nsequence=%s\nstate_copied=false\n' \
    "$_ub_old" "$_ub_new" "$_ub_seq" > "$_ub_tmp/snapshot.info" || exit 1
  printf '%s\n' tailscaled-upgrade-backup-v1 > "$_ub_tmp/.complete" || exit 1
  find "$_ub_tmp" -type d -exec chmod 0700 {} \; || exit 1
  find "$_ub_tmp" -type f -exec chmod 0600 {} \; || exit 1
  chown -R 0:0 "$_ub_tmp" || exit 1
  mv "$_ub_tmp" "$_ub_dest" || exit 1
  trap - 0 1 2 15
  # Prune only completed, format-owned generations. Paths have no whitespace.
  for _ub_version in "$_ub_root"/*; do
    [ -d "$_ub_version" ] || continue
    case "${_ub_version##*/}" in .|..|*[!a-zA-Z0-9._-]*) continue ;; esac
    for _ub_snap in "$_ub_version"/*; do
      [ -d "$_ub_snap" ] || continue
      case "${_ub_snap##*/}" in ''|*[!0-9]*) continue ;; esac
      [ "$(cat "$_ub_snap/.complete" 2>/dev/null)" = tailscaled-upgrade-backup-v1 ] || continue
      printf '%s %s\n' "${_ub_snap##*/}" "$_ub_snap"
    done
  done | sort -rn | awk 'NR>5 {print $2}' | while IFS= read -r _ub_prune; do
    rm -rf "$_ub_prune" || exit 1
    rmdir "${_ub_prune%/*}" 2>/dev/null || :
  done || exit 1
  ui_print "- Private upgrade backup: $_ub_dest (latest 5 retained; no state copy)"
)
backup_upgrade || abort "! Upgrade backup failed; existing service/config were not replaced"

if [ -f "$INSTALL_DIR/scripts/tailscaled.service" ]; then
  ui_print "- Stopping the running tailscaled service"
  "$INSTALL_DIR/scripts/tailscaled.service" stop >/dev/null 2>&1
fi

# Belt and braces. The binary is replaced with cp -f, i.e. in place, so a daemon
# that another module started keeps executing the OLD code while the file on disk
# (and its hash) is already ours. Kill anything still using our state directory
# regardless of which module launched it.
for _p in $(pgrep -f "statedir=${INSTALL_DIR}/run" 2>/dev/null); do
  kill -15 "$_p" 2>/dev/null
done
sleep 1
for _p in $(pgrep -f "statedir=${INSTALL_DIR}/run" 2>/dev/null); do
  ui_print "  killing stale tailscaled pid $_p"
  kill -9 "$_p" 2>/dev/null
done

ui_print "- Creating directories"
mkdir -p "$INSTALL_DIR" "$INSTALL_BIN_DIR" "$SERVICE_DIR"

# The combined binary serves as both the daemon and the CLI.
ui_print "- Installing binaries"
mv -f "$MODPATH/files/tailscale.combined" "$INSTALL_BIN_DIR/tailscale"
cp -f "$INSTALL_BIN_DIR/tailscale" "$INSTALL_BIN_DIR/tailscaled"
mv -f "$MODPATH/files/android-dns" "$INSTALL_BIN_DIR/android-dns"
mv -f "$MODPATH/files/android-hostname" "$INSTALL_BIN_DIR/android-hostname"
mv -f "$MODPATH/files/android-netdiag" "$INSTALL_BIN_DIR/android-netdiag"
[ -f "$MODPATH/files/build-info.json" ] && cp -f "$MODPATH/files/build-info.json" "$INSTALL_DIR/build-info.json"

# Keep a known-good copy of the binary plus its checksum. `tailscale update`
# downloads the OFFICIAL upstream release, which is compiled for GOOS=linux and
# therefore activates Tailscale's osrouter (fwmark/lookup main/ts-* iptables
# chains) that cannot work on Android - that is what blocks the link. The
# service compares the hashes on every start and restores this copy.
ui_print "- Storing the known-good binary copy"
cp -f "$INSTALL_BIN_DIR/tailscaled" "$INSTALL_BIN_DIR/tailscaled.orig"
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "$INSTALL_BIN_DIR/tailscaled.orig" | awk '{print $1}' > "$INSTALL_BIN_DIR/tailscaled.sha256"
elif command -v busybox >/dev/null 2>&1; then
  busybox sha256sum "$INSTALL_BIN_DIR/tailscaled.orig" | awk '{print $1}' > "$INSTALL_BIN_DIR/tailscaled.sha256"
fi
[ -s "$INSTALL_BIN_DIR/tailscaled.sha256" ] || ui_print "! could not compute the checksum (guarded by size instead)"

# Scripts / settings. Copy without overwriting an existing settings.ini or routes
# file so local edits survive an update.
ui_print "- Installing scripts"
mkdir -p "$INSTALL_DIR/scripts"
cp -f "$MODPATH"/tailscale/scripts/* "$INSTALL_DIR/scripts/"
for f in settings.ini routes; do
  if [ -f "$INSTALL_DIR/$f" ]; then
    ui_print "  keeping existing $INSTALL_DIR/$f"
  else
    cp -f "$MODPATH/tailscale/$f" "$INSTALL_DIR/$f" 2>/dev/null
  fi
done

# Track the installed module edition for the next upgrade's source-version label.
if [ -f "$MODPATH/module.prop" ]; then
  (umask 077; cp -f "$MODPATH/module.prop" "$INSTALL_DIR/installed-module.prop" && \
    chmod 0600 "$INSTALL_DIR/installed-module.prop") || abort "! Could not record installed module version"
fi

rm -rf "$MODPATH/files" "$MODPATH/tailscale"

ui_print "- Setting permissions"
# Scope permissions to installed program files. Never chmod existing node keys,
# state, routes, or user settings on upgrade.
set_perm_recursive "$INSTALL_BIN_DIR" 0 0 0755 0755
set_perm_recursive "$INSTALL_DIR/scripts" 0 0 0755 0755
set_perm_recursive "$MODPATH/system/bin" 0 0 0755 0755
# KernelSU / APatch read the WebUI from the module directory itself.
[ -d "$MODPATH/webroot" ] && set_perm_recursive "$MODPATH/webroot" 0 0 0755 0644
set_perm "$MODPATH/service.sh" 0 0 0755
mv -f "$MODPATH/service.sh" "$SERVICE_DIR/tailscaled_service.sh"

ui_print "-----------------------------------------------------------"
ui_print " Instructions"
ui_print "-----------------------------------------------------------"
ui_print "- Reboot the device."
ui_print "- Open this module's card in the manager for the WebUI:"
ui_print "  Status / Routes / Features / Log / Diagnostics"
ui_print ""
ui_print "- Or from a terminal:"
ui_print "  su -c 'tailscaled.service start'"
ui_print "  su -c 'tailscale login'"
ui_print "  su -c 'tailscaled.service diag'"
ui_print ""
ui_print "- To reach a peer's advertised subnet (e.g. 192.168.100.0/24):"
ui_print "  su -c 'tailscale set --accept-routes'"
ui_print "  (the route must also be approved in the admin console)"
ui_print ""
ui_print "! Do NOT run 'tailscale update': it installs the official"
ui_print "! GOOS=linux binary, which cannot work here. The service"
ui_print "! detects and reverts it, but simply avoid it."
