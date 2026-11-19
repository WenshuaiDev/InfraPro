import { createHash } from 'node:crypto';
import { isIP } from 'node:net';
import { httpOwners,inventory, managed } from './architecture-runtime.mjs';

const publicPorts={api:8080,'display-runtime':5180,'display-assets':5181,parser:8091};
export const publicVars={api:'INFRAPRO_HTTP_ADDR','display-runtime':'INFRAPRO_DISPLAY_RUNTIME_ADDR','display-assets':'INFRAPRO_DISPLAY_ASSET_ADDR',parser:'INFRAPRO_PARSER_ADDR'};
const hostPort=(host,port)=> (isIP(host)===6?'['+host+']':host)+':'+port;
function validateHost(host){
  if(typeof host!=='string'||(!isIP(host)&&!/^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)*[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/i.test(host)))throw Error('Invalid replica host');
  return host;
}
function share(total,index,count){return Math.floor(total/count)+(index<total%count?1:0);}

// A role is the protocol identity. A slot is a separately provisioned process
// with its own key, login, report directory, listener and resource allocation.
export function replicaLayout({mode,host,port,replicas={},maxInflight={},publicListeners={},containerRoles=new Set()}){
  const docker=mode==='compose',reachable=validateHost(host??'127.0.0.1');
  const roles=[...managed,'parser','ingest-relay'];
  for(const [role,value] of Object.entries(maxInflight)){
    if(!roles.includes(role)||!Number.isInteger(value)||value<1||value>16384||(role==='parser'&&value>4))
      throw Error('Invalid in-flight role budget for '+role);
  }
  const budget=role=>maxInflight[role]??inventory.roles[role].maxInflight;
  for(const name of Object.keys(replicas))if(!roles.includes(name))throw Error('Unknown replica role '+name);
  for(const [name,address] of Object.entries(publicListeners)){
    if(!(name in publicPorts)||typeof address!=='string')throw Error('Unknown public listener '+name);
  }
  const reserved=new Set([...Object.keys(inventory.roles),'web','broker-1','broker-2','broker-3','replica-proxy']);
  const endpoints={},instances={},slots={},proxies=[],sockets=new Set();
  const portOf=new Map(Object.keys(inventory.privatePorts).map((role,i)=>[role,port+i]));
  for(const [role,n] of portOf){
    endpoints[role]='https://'+hostPort(docker?(host??role):reachable,n);
    sockets.add(n);
  }
  const publicEndpoints={};
  for(const [role,def] of Object.entries(publicPorts)){
    const address=publicListeners[role]??hostPort('127.0.0.1',def);
    const parsed=new URL('http://'+address);
    if(parsed.username||parsed.password||parsed.pathname!=='/'||parsed.search||parsed.hash)throw Error('Invalid public listener '+role);
    const hostname=validateHost(parsed.hostname.replace(/^\[|\]$/g,'')),n=Number(parsed.port||80);
    if(sockets.has(n))throw Error('Overlapping service listener '+n);
    publicEndpoints[role]={host:hostname,port:n};sockets.add(n);
  }
  let nextPort=port+portOf.size;
  function allocate(){
    while(sockets.has(nextPort))nextPort++;
    if(nextPort>65535)throw Error('Insufficient listener ports for replicas');
    sockets.add(nextPort);return nextPort++;
  }
  for(const role of roles){
    const spec=inventory.roles[role],declarations=Object.hasOwn(replicas,role)?replicas[role]:[role+'-1'];
    const inflightLimit=budget(role);
    if(!Array.isArray(declarations)||!declarations.length||declarations.length>128||declarations.length>inflightLimit||
      (spec.pgConnections>0&&declarations.length>spec.pgConnections))throw Error('Replica count exceeds quota for '+role);
    if(role==='ingest-relay'&&2*declarations.length>spec.pgConnections)throw Error('Every cloud spool needs a lock connection and a business connection');
    const n=declarations.length,multi=n>1;
    slots[role]=[];instances[role]=[];
    for(const [index,value] of declarations.entries()){
      const slot=typeof value==='string'?{id:value}:value;
      if(!slot||Object.keys(slot).some(k=>!['id','host','diagnostic_node'].includes(k))||typeof slot.id!=='string'||!/^[a-z][a-z0-9-]{0,62}$/.test(slot.id)||reserved.has(slot.id))
        throw Error('Invalid or duplicate replica identity for '+role);
      const backend=validateHost(slot.host??'127.0.0.1');
      if(docker&&slot.host)throw Error('Compose replicas use private service DNS, omit slot host');
      reserved.add(slot.id);slots[role].push(slot.id);
      const label=multi?slot.id:role;
      const instance={role,id:slot.id,label,directory:index===0?'roles/'+role:'replicas/'+role+'/'+slot.id,systemUser:
        multi?'ip-'+createHash('sha256').update(role+'/'+slot.id).digest('hex').slice(0,15):spec.systemUser??(role==='parser'?'infrapro-parser':'infrapro-relay'),
        pgConnections:share(spec.pgConnections,index,n),maxInflight:share(inflightLimit,index,n),cpu:spec.cpu/n,
        memoryMiB:share(spec.memoryMiB,index,n),tempMiB:share(spec.tempMiB,index,n)};
      if(slot.host)instance.host=backend;
      const nodeID=/^[a-z][a-z0-9_.-]{0,63}$/;
      if(slot.diagnostic_node!==undefined&&(typeof slot.diagnostic_node!=='string'||!nodeID.test(slot.diagnostic_node)))
        throw Error('Invalid diagnostic node for '+slot.id);
      const defaultNode=docker?(role==='ingest-relay'?'edge':'docker-observer'):
        mode==='mixed'&&containerRoles.has(role)?'docker-observer':'local';
      const hostNode=slot.host&&(nodeID.test(backend)?backend:'node-'+createHash('sha256').update(backend).digest('hex').slice(0,16));
      instance.diagnostic_node=slot.diagnostic_node??(hostNode||defaultNode);
      if(portOf.has(role)){
        const p=multi&&!docker?allocate():portOf.get(role);
        instance.listen_address=hostPort(docker?'0.0.0.0':multi?backend:reachable,p);
        if(multi&&!docker)instance.private_backend={host:backend,port:p};
      }
      if(publicPorts[role]&&(multi||docker||publicListeners[role])){
        const p=multi&&!docker?allocate():publicEndpoints[role].port;
        instance.public_address=hostPort(docker?'0.0.0.0':multi?backend:publicEndpoints[role].host,p);
        if(multi&&!docker)instance.public_backend={host:backend,port:p};
      }
      instances[role].push(instance);
    }
    if(multi&&!docker&&portOf.has(role))proxies.push({role,protocol:'private-tls',max_connections:inflightLimit,listen:{host:reachable,port:portOf.get(role)},backends:instances[role].map(i=>i.private_backend)});
    if(multi&&!docker&&publicPorts[role])proxies.push({role,protocol:'external-http',max_connections:inflightLimit,listen:publicEndpoints[role],backends:instances[role].map(i=>i.public_backend)});
  }
  if(httpOwners.some(owner=>slots.api.length>budget(owner)))throw Error('Gateway slot count exceeds a downstream owner budget');
  return {endpoints,instances,slots,proxies,publicEndpoints};
}
