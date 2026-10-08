export function exportText(input, explicitSecrets = []) {
 if(typeof input!=="string" || !/^Tailscaled-fix Diagnostic Report\r?\n/.test(input) || !/^redaction: enabled\r?$/m.test(input))throw new Error("缺少脱敏报告标记");
 let report=input;
 explicitSecrets.filter(v=>typeof v==="string" && v.length>0).sort((a,b)=>b.length-a.length).forEach(secret=>{report=report.split(secret).join("[REDACTED]");});
 return report;
}

// KernelSU WebView may ignore Blob downloads. Only an explicitly requested,
// redaction-marked report is exported; raw report text never enters shell syntax.
export async function saveNativeReport(input, execute, stamp) {
 const report=exportText(input);
 if(!/^[0-9]{8}T[0-9]{9}Z$/.test(stamp))throw new Error("无效报告文件名");
 const bytes=new TextEncoder().encode(report);
 if(bytes.length>1024*1024)throw new Error("报告过大，请复制文本");
 let binary="";for(let offset=0;offset<bytes.length;offset+=8192)binary+=String.fromCharCode(...bytes.subarray(offset,offset+8192));
 const encoded=btoa(binary), dir="/data/adb/tailscale/run/ui-report-export/"+stamp;
 const file="/sdcard/Download/tailscaled-diagnostic-"+stamp+".txt", part=file+".part";
 let owned=false;
 async function command(text){const result=await execute(text);if(!result || result.errno!==0)throw new Error("报告写入失败，可复制文本");}
 try {
  await command(`umask 077; mkdir -p /data/adb/tailscale/run/ui-report-export && mkdir '${dir}'`);owned=true;
  for(let offset=0;offset<encoded.length;offset+=12000)await command(`umask 077; printf '%s' '${encoded.slice(offset,offset+12000)}' >> '${dir}/data.b64'`);
  await command(`test ! -e '${file}' && test ! -e '${part}' && (set -C; : > '${part}' || exit 1; set +C; trap "rm -f '${part}'" EXIT; /system/bin/base64 -d '${dir}/data.b64' > '${part}' && mv -n '${part}' '${file}' && test ! -e '${part}' && test "$(wc -c < '${file}')" -eq ${bytes.length})`);
  return file;
 } finally {
  if(owned){try{await execute(`rm -f '${dir}/data.b64'; rmdir '${dir}'`);}catch(_){/* Preserve the original write error. */}}
 }
}
