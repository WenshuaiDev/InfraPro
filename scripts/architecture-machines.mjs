import { readFileSync } from 'node:fs';
import { join,resolve } from 'node:path';
import { inventory,root } from './architecture-runtime.mjs';
import { processEnvironment } from './architecture-process-resources.mjs';

export function boundedRelayConfig(input,slot){
  const url=new URL(input.database_url);url.searchParams.set('pool_max_conns',String(slot.pgConnections));
  return {...input,database_url:url.href,max_connections:Math.min(input.max_connections??slot.maxInflight,slot.maxInflight)};
}

export function renderRelaySlots(path,slots,input,save){
  const sources=input?JSON.parse(readFileSync(resolve(input))):{},spools=new Set(),identities=new Set(),logins=new Set();
  const ids=new Set(slots.map(i=>i.id));
  if(Object.keys(sources).some(id=>!ids.has(id)))throw Error('Unknown cloud relay slot');
  for(const slot of slots){
    const file=sources[slot.id];
    if(slots.length>1&&!file)throw Error('Multiple independent relay installations require their individual spool configurations');
    const source=file?resolve(file):join(root,'deploy/ingestion/relay.example.json');
    const config=boundedRelayConfig(JSON.parse(readFileSync(source)),slot),url=new URL(config.database_url);
    if(!['postgresql:','postgres:'].includes(url.protocol)||!url.username||!config.relay_id)throw Error('Invalid cloud relay database/identity');
    const settings=url.searchParams.get('search_path')??url.searchParams.get('options')?.match(/(?:^|\s)-c\s*search_path=(\S+)/)?.[1]??'public';
    const primary=settings.split(',')[0].trim().replace(/^"|"$/g,'');
    const endpoint=url.hostname.toLowerCase()+':'+(url.port||'5432');
    const spool=endpoint+decodeURIComponent(url.pathname)+'/'+primary,login=endpoint+'/'+decodeURIComponent(url.username);
    if(spools.has(spool)||identities.has(config.relay_id)||logins.has(login))throw Error('Cloud relay slots must each own an independent spool, RelayID and database login');
    identities.add(config.relay_id);spools.add(spool);logins.add(login);
    const dir=join(path,slot.directory),name=file?'relay.json':'relay.example.json';
    save(join(dir,name),config);
    const user=decodeURIComponent(url.username);
    if(!/^[a-zA-Z_][a-zA-Z0-9_-]{0,62}$/.test(user))throw Error('Invalid cloud relay login');
    save(join(dir,'postgres-limit.sql'),"DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='"+user+"' AND rolcanlogin AND NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls OR rolreplication)) THEN RAISE EXCEPTION 'An independent unprivileged spool login is required'; END IF; END $$;\n"+'ALTER ROLE "'+user+'" CONNECTION LIMIT '+slot.pgConnections+';\n');
    const unit=readFileSync(join(root,'deploy/ingestion/systemd/infrapro-ingest-relay.service'),'utf8')
      .replace('User=infrapro-relay','User='+slot.systemUser).replace('Group=infrapro-relay','Group='+slot.systemUser)
      .replace('/opt/infrapro/bin/ingest-relay serve /etc/infrapro-relay/relay.json','/opt/infrapro/current/bin/ingest-relay serve /etc/infrapro/'+slot.directory+'/relay.json')
      .replace('INFRAPRO_DIAGNOSTICS_REPLICA_ID=relay-1','INFRAPRO_DIAGNOSTICS_REPLICA_ID='+slot.id)
      .replace('INFRAPRO_DIAGNOSTICS_NODE_ID=cloud-node-1','INFRAPRO_DIAGNOSTICS_NODE_ID='+slot.diagnostic_node)
      .replace('INFRAPRO_DIAGNOSTICS_INSTANCE_ID=cloud-relay','INFRAPRO_DIAGNOSTICS_INSTANCE_ID=infrapro-architecture')
      .replace('TimeoutStopSec=30',['TimeoutStopSec='+inventory.roles['ingest-relay'].stopSeconds,...Object.entries(processEnvironment(slot)).map(([k,v])=>'Environment='+k+'='+v),'CPUQuota='+(slot.cpu*100).toFixed(2)+'%','MemoryMax='+slot.memoryMiB+'M','TemporaryFileSystem=/tmp:rw,size='+slot.tempMiB+'M,mode=1777','TasksMax=128'].join('\n'))
      .replaceAll('/run/infrapro-relay','/run/infrapro-'+slot.label).replace('RuntimeDirectory=infrapro-relay','RuntimeDirectory=infrapro-'+slot.label);
    save(join(path,'edge/units/infrapro-'+slot.label+'.service'),unit,0o644);
  }
}
