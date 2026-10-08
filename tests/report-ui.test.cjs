const assert=require("node:assert/strict"),fs=require("node:fs");
(async()=>{const m=await import("data:text/javascript;base64,"+fs.readFileSync("webroot/report.js").toString("base64"));
 const text="Tailscaled-fix Diagnostic Report\nredaction: enabled\n100.72.239.86 ccmni1 CUSTOM_CANARY CUSTOM_CANARY\n";
 const out=m.exportText(text,["CUSTOM_CANARY"]);assert.ok(!out.includes("CUSTOM_CANARY"));assert.ok(out.includes("100.72.239.86 ccmni1"));
 for(const bad of [null,"raw token", "Tailscaled-fix Diagnostic Report\nraw secret",{}])assert.throws(()=>m.exportText(bad));
 assert.equal(m.exportText(text,["",null]).includes("CUSTOM_CANARY"),true);
 const saved=[];const unicode=m.exportText(text,['CUSTOM_CANARY'])+'中文报告\n';
 const target=await m.saveNativeReport(unicode,async command=>{saved.push(command);return {errno:0};},'20261008T080000000Z');
 assert.equal(target,'/sdcard/Download/tailscaled-diagnostic-20261008T080000000Z.txt');
 assert(!saved.join('\n').includes('CUSTOM_CANARY'));assert(!saved.join('\n').includes('中文报告'));
 const encoded=saved.filter(c=>c.includes('printf')).map(c=>c.match(/'([A-Za-z0-9+/=]+)'/)[1]).join('');
 assert.equal(Buffer.from(encoded,'base64').toString(),unicode);
 let calls=0;await assert.rejects(()=>m.saveNativeReport(unicode,async()=>{calls++;return {errno:1};},'20261008T080000001Z'));assert.equal(calls,1);
 await assert.rejects(()=>m.saveNativeReport(unicode,async()=>{throw Error('bridge timeout')},'20261008T080000002Z'));
 await assert.rejects(()=>m.saveNativeReport(unicode,async()=>({errno:0}),'$(id)'));
 // Exercise shell failure after the public .part has actually been created.
 if(process.platform!=='win32') {
 const os=require('os'),path=require('path'),cp=require('child_process');
 const temp=fs.mkdtempSync(path.join(os.tmpdir(),'tailscaled-report-')).replace(/\\/g,'/');fs.mkdirSync(temp+'/Download');
 const translate=c=>c.split('/data/adb/tailscale/run/ui-report-export').join(temp+'/private').split('/sdcard/Download').join(temp+'/Download').split('/system/bin/base64').join('false');
 try {
  await assert.rejects(()=>m.saveNativeReport(unicode,async c=>{const r=cp.spawnSync('sh',['-c',translate(c)]);assert.ifError(r.error);return {errno:r.status};},'20261008T080000003Z'));
  assert.deepEqual(fs.readdirSync(temp+'/Download'),[], 'failed decode must not leave a partial export');
  fs.writeFileSync(temp+'/Download/tailscaled-diagnostic-20261008T080000004Z.txt.part','foreign');
  await assert.rejects(()=>m.saveNativeReport(unicode,async c=>{const r=cp.spawnSync('sh',['-c',translate(c)]);assert.ifError(r.error);return {errno:r.status};},'20261008T080000004Z'));
  assert.equal(fs.readFileSync(temp+'/Download/tailscaled-diagnostic-20261008T080000004Z.txt.part','utf8'),'foreign');
 } finally {fs.rmSync(temp,{recursive:true,force:true});}
 } else console.log('POSIX export filesystem tests run separately under WSL');
 console.log("PASS report explicit-secret canary removal and unsupported raw output rejection (3 groups)");
})().catch(e=>{console.error(e);process.exitCode=1});
