// Manager root shells can use a different PATH and mount namespace from an
// interactive su shell. Installed data/module files do not need system overlays.
const SERVICE = "/data/adb/tailscale/scripts/tailscaled.service";
const CLI_WRAPPER = "/data/adb/modules/tailscaled/system/bin/tailscale";
const SHELL_PATH =
  'export PATH="/system/bin:/system/xbin:/vendor/bin:/data/adb/ksu/bin:/data/adb/magisk:$PATH"\n';

export function nativeCommand(command) {
  let installed = command;
  if (command.indexOf("tailscaled.service ") === 0) {
    installed = SERVICE + command.slice("tailscaled.service".length);
  } else if (command.indexOf("tailscale ") === 0) {
    // Keep the original wrapper's settings.ini, custom socket and update guard.
    installed = CLI_WRAPPER + command.slice("tailscale".length);
  }
  return SHELL_PATH + installed;
}
