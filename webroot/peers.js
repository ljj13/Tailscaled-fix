// Presentation-only adapter for status JSON. Never infer an active path from home relay.
const obj = v => v && typeof v === "object" && !Array.isArray(v) ? v : {};
const str = v => typeof v === "string" ? v : "";
const arr = v => Array.isArray(v) ? v.filter(x => typeof x === "string") : [];
const bool = v => v === true ? true : v === false ? false : null;
function ipFamily(ip) {
 if (/^100\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(ip) && ip.split('.').every(x=>Number(x)<=255) && Number(ip.split('.')[1])>=64 && Number(ip.split('.')[1])<=127) return 4;
 if (/^fd7a:[0-9a-f:]+$/i.test(ip) && ip.length<=39) {
  const parts=ip.split('::');
  if(parts.length<=2) {
   const groups=parts.map(part=>part?part.split(':'):[]);
   const count=groups.reduce((n,g)=>n+g.length,0);
   if(groups.every(g=>g.every(x=>/^[0-9a-f]{1,4}$/i.test(x))) && (parts.length===2 ? count<8 : count===8)) return 6;
  }
 }
 return 0;
}
export function peerList(input) {
 const status=obj(input), self=obj(status.Self), ips=arr(self.TailscaleIPs);
 const peers=Array.isArray(status.Peer) ? status.Peer : Object.values(obj(status.Peer));
 return peers.filter(p=>p && typeof p==='object' && !Array.isArray(p)).filter(p=>
  !(str(self.ID) && p.ID===self.ID) && !(str(self.PublicKey) && p.PublicKey===self.PublicKey) && !arr(p.TailscaleIPs).some(ip=>ips.includes(ip))
 ).map((p,index)=>{
  const addresses=arr(p.TailscaleIPs), online=bool(p.Online), active=p.Active===true && online!==false;
  const endpoint=str(p.CurAddr), relay=str(p.Relay);
  const subnets=arr(p.PrimaryRoutes || p.AdvertisedRoutes || p.AllowedIPs).filter(r=>{
   const ip=r.split('/')[0];return r.includes('/') && !ipFamily(ip) && r!=='0.0.0.0/0' && r!=='::/0';
  });
  return {id:str(p.ID)||String(index),hostname:str(p.HostName)||str(p.DNSName).replace(/\.$/,'')||'未知设备',os:str(p.OS)||'未知',online,
   ipv4:addresses.filter(ip=>ipFamily(ip)===4),ipv6:addresses.filter(ip=>ipFamily(ip)===6),
   path:active && endpoint ? 'Direct' : active && relay && !p.PeerRelay ? 'DERP' : 'Unknown',endpoint:active?endpoint:'',relay,
   peerRelay:str(p.PeerRelay),lastSeen:str(p.LastSeen)||str(p.LastWrite)||str(p.LastHandshake),
   exitNode:bool(p.ExitNodeOption),subnets};
 }).sort((a,b)=>a.hostname.localeCompare(b.hostname));
}
export function peerPingCommand(ip) {
 if (!ipFamily(ip)) throw new Error('无有效 Tailnet 地址');
 return `tailscale ping --timeout=3s --c=3 --until-direct=false ${ip}`;
}
export function pingSummary(output) {
 const lines=String(output).split('\n').filter(line=>/pong .* via .* in /.test(line));
 const last=lines[lines.length-1] || '', match=last.match(/ via (.+) in ([0-9.]+(?:ms|s))/);
 return {path:match ? (/^DERP\(/.test(match[1]) ? match[1] : 'Direct · '+match[1]) : 'Unknown',rtt:match?match[2]:'未取得 RTT'};
}
