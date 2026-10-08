// Some managers expose a permanently light WebView even with Android night on.
// Read only the computed system mode; never change system or service settings.
export const ANDROID_THEME_COMMAND = "/system/bin/timeout 2 /system/bin/dumpsys uimode";
export function parseAndroidTheme(result) {
  if (!result || result.errno !== 0) return null;
  const values = String(result.stdout || "").match(/\bmComputedNightMode=(?:true|false)\b/g) || [];
  if (!values.length || values.some((value) => value !== values[0])) return null;
  return values[0].endsWith("=true");
}
export function createThemeSync({ media, readSystem, apply, timeoutMs = 2500 }) {
  let verified = null, current = null, reading = false;
  function render() {
    const dark = verified === null ? Boolean(media()) : verified;
    if (current !== dark) { current = dark; apply(dark ? "dark" : "light"); }
  }
  render();
  async function refresh() {
    render();
    if (!readSystem || reading) return;
    reading = true;
    let timer;
    const request = Promise.resolve().then(readSystem).then(
      (value) => { reading = false; return value; },
      () => { reading = false; return null; },
    );
    const result = await Promise.race([
      request,
      new Promise((resolve) => { timer = setTimeout(() => resolve(null), timeoutMs); }),
    ]);
    clearTimeout(timer);
    const next = parseAndroidTheme(result);
    if (next !== null) verified = next;
    render();
  }
  return { refresh, current: () => current };
}
