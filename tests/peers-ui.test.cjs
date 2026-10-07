const assert = require("node:assert/strict");
const fs = require("node:fs");
(async () => {
 const m = await import("data:text/javascript;base64," + fs.readFileSync("webroot/peers.js").toString("base64"));
 const self={ID:"self",TailscaleIPs:["100.64.1.1"]};
 const fixture={Self:self,Peer:{self:{...self,Online:true},a:{HostName:"online",Online:true,Active:true,CurAddr:"[2001:db8::1]:41641",TailscaleIPs:["100.72.239.86","fd7a:115c:a1e0::1"],ExitNodeOption:true,PrimaryRoutes:["192.168.1.0/24"]},b:{HostName:"offline",Online:false,Active:true,CurAddr:"stale",Relay:"hkg"},c:{HostName:"idle",Online:true,Relay:"hkg"},d:{HostName:"relay",Online:true,Active:true,Relay:"hkg"}}};
 const peers=m.peerList(fixture);assert.equal(peers.length,4);assert.equal(peers.find(p=>p.hostname==='online').path,'Direct');assert.equal(peers.find(p=>p.hostname==='offline').path,'Unknown');assert.equal(peers.find(p=>p.hostname==='idle').path,'Unknown');assert.equal(peers.find(p=>p.hostname==='relay').path,'DERP');assert.equal(peers.find(p=>p.hostname==='online').exitNode,true);assert.deepEqual(peers.find(p=>p.hostname==='online').subnets,['192.168.1.0/24']);
 for(const bad of [null,{}, {Peer:null},{Peer:[null,false,{}]}]) assert.doesNotThrow(()=>m.peerList(bad));
 assert.equal(m.peerList({Self:{TailscaleIPs:['100.64.1.1']},Peer:{x:{TailscaleIPs:['100.64.1.1']}}}).length,0);
 for(const bad of ['--help','100.1.1.1;id','$(id)','fd7a::1\nreboot','hostname'])assert.throws(()=>m.peerPingCommand(bad));
 assert.match(m.peerPingCommand('100.72.239.86'),/--timeout=3s/);
 assert.match(m.pingSummary('pong from host via DERP(hkg) in 332ms').path,/DERP/);
 assert.equal(m.pingSummary('pong from host via [2001:db8::1]:41641 in 32ms').rtt,'32ms');
 assert.equal(m.pingSummary('timeout').path,'Unknown');
 console.log('PASS peers normalization, self exclusion, paths/capabilities, hostile targets and ping parsing (8 groups)');
})().catch(e=>{console.error(e);process.exitCode=1});
