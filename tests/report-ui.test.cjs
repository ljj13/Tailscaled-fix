const assert=require("node:assert/strict"),fs=require("node:fs");
(async()=>{const m=await import("data:text/javascript;base64,"+fs.readFileSync("webroot/report.js").toString("base64"));
 const text="Tailscaled-fix Diagnostic Report\nredaction: enabled\n100.72.239.86 ccmni1 CUSTOM_CANARY CUSTOM_CANARY\n";
 const out=m.exportText(text,["CUSTOM_CANARY"]);assert.ok(!out.includes("CUSTOM_CANARY"));assert.ok(out.includes("100.72.239.86 ccmni1"));
 for(const bad of [null,"raw token", "Tailscaled-fix Diagnostic Report\nraw secret",{}])assert.throws(()=>m.exportText(bad));
 assert.equal(m.exportText(text,["",null]).includes("CUSTOM_CANARY"),true);
 console.log("PASS report explicit-secret canary removal and unsupported raw output rejection (3 groups)");
})().catch(e=>{console.error(e);process.exitCode=1});
