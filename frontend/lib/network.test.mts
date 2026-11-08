import test from 'node:test';
import assert from 'node:assert/strict';
import { detectAccess, emptyNetworkSettings, isPrivateHost, serviceEndpoint } from './network.ts';
test('private hosts cover RFC1918 and IPv6 without treating public IPs as private',()=>{
 for(const h of ['192.168.1.1','10.1.1.1','172.16.0.1','172.31.255.254','127.0.0.1','portal.local','localhost','[::1]','fd12::1','fe80::1'])assert.equal(isPrivateHost(h),true,h);
 for(const h of ['172.32.0.1','172.15.0.1','8.8.8.8','public.example.com','192.168.999.1','2001:4860::1'])assert.equal(isPrivateHost(h),false,h);
});
test('explicit portal rules win and a public hostname never implies LAN reachability',()=>{
 const settings={...emptyNetworkSettings,internalHosts:['portal.lan'],externalHosts:['portal.local']};
 assert.equal(detectAccess('PORTAL.LAN.',settings).scope,'internal');
 assert.equal(detectAccess('portal.local',settings).scope,'external');
 assert.equal(detectAccess('public.example.com',settings).scope,'external');
 assert.equal(detectAccess('192.168.1.10',settings).scope,'internal');
});
test('legacy public URLs survive migration and a missing endpoint never silently crosses networks',()=>{
 const s={id:'1',name:'test',tenant:'platform',category:'test',status:'unverified',description:'',url:'https://public.example.com'};
 assert.equal(serviceEndpoint(s,'external'),s.url);
 assert.equal(serviceEndpoint(s,'internal'),'');
 assert.equal(serviceEndpoint({...s,internalUrl:'http://service.local'},'internal'),'http://service.local');
 assert.equal(serviceEndpoint({...s,internalUrl:'javascript:alert(1)'},'internal'),'');
 assert.equal(serviceEndpoint({...s,externalUrl:'https://user:pass@example.com'},'external'),'');
});
