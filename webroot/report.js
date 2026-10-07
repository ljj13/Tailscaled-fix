export function exportText(input, explicitSecrets = []) {
 if(typeof input!=="string" || !/^Tailscaled-fix Diagnostic Report\r?\n/.test(input) || !/^redaction: enabled\r?$/m.test(input))throw new Error("缺少脱敏报告标记");
 let report=input;
 explicitSecrets.filter(v=>typeof v==="string" && v.length>0).sort((a,b)=>b.length-a.length).forEach(secret=>{report=report.split(secret).join("[REDACTED]");});
 return report;
}
