import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

// The NATS config grammar accepts quoted JSON scalar values. Only these
// generated allowlists vary by restore epoch; role names are certificate SANs.
const destinations={
  'parsing-dispatch-worker':{identify:'ingestion.receipt.stored',plan:'ingestion.units.ready',parse:'ingestion.plan.ready'},
  'monitoring-incremental-worker':{summary:'monitoring.changed',derived:'monitoring.changed'},
  'monitoring-read-worker':{'query-cache':'monitoring.changed','display-cache':'monitoring.changed'},
  'task-index-worker':{'task-index':'task.projection.ready'},
  'notification-worker':{notification:'notification.intent.ready'},
  'audit-worker':{audit:'audit.work.ready'},
};

export function render(input, epoch){
  const {environment,name,listen,cluster_listen,routes=[],ca_file,cert_file,key_file,route_ca_file,route_cert_file,route_key_file,store_dir,max_file_bytes=4294967296,monitor_port=8222}=input;
  if(!/^[a-z][a-z0-9_]{0,31}$/.test(environment)||!/^[a-zA-Z][\w-]{0,63}$/.test(name)||!/^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/.test(epoch))throw Error('Invalid server name, environment or restore epoch');
  for(const path of [ca_file,cert_file,key_file,store_dir,...(routes.length?[route_ca_file,route_cert_file,route_key_file]:[])])if(typeof path!=='string'||!path.startsWith('/'))throw Error('Certificate and volume paths must be absolute');
  if(!Number.isSafeInteger(max_file_bytes)||max_file_bytes<65536||!Number.isInteger(monitor_port)||monitor_port<1||monitor_port>65535)throw Error('Invalid broker capacity or monitoring port');
  const q=JSON.stringify;
  const root='infrapro.'+environment+'-'+epoch+'.v1.';
  const stream=f=>'IP_'+environment+'_'+epoch.replaceAll('-','')+'_'+f;
  const family=t=>t.startsWith('ingestion.')?'ingestion':t==='monitoring.changed'?'monitoring':'auxiliary';
  const inbox=n=>'ipreply.'+environment+'.'+n+'.>';
  function user(name,pub,sub){
    return '{ user: '+q(name+'.infrapro.internal')+', permissions: { publish: '+q(pub)+', subscribe: '+q(sub)+' } }';
  }
  const topics=[...new Set(Object.values(destinations).flatMap(table=>Object.values(table)))];
  const users=[
    user('event-admin',['$JS.API.>'],[inbox('admin')]),
    user('event-relay',[...topics.map(topic=>root+topic),...['ingestion','monitoring','auxiliary'].map(f=>'$JS.API.STREAM.INFO.'+stream(f))],[inbox('relay')]),
  ];
  for(const [role,groups] of Object.entries(destinations)){
    const publish=[],subscribe=[];
    for(const [group,topic] of Object.entries(groups)){
      const name=stream(family(topic));
      for(const consumer of [group,group+'-advisories']){
        const target=name+'.'+consumer;
        publish.push('$JS.API.CONSUMER.INFO.'+target,'$JS.API.CONSUMER.MSG.NEXT.'+target,'$JS.ACK.'+target+'.>','$JS.ACK.*.*.'+target+'.>');
      }
      subscribe.push(inbox(group));
    }
    users.push(user(role,publish,subscribe));
  }
  let result='server_name: '+q(name)+'\nlisten: '+q(listen)+'\nhttp: '+q('127.0.0.1:'+monitor_port)+'\n'+
    'jetstream { store_dir: '+q(store_dir)+', max_file_store: '+max_file_bytes+' }\n'+
    'tls { cert_file: '+q(cert_file)+', key_file: '+q(key_file)+', ca_file: '+q(ca_file)+', verify_and_map: true, min_version: "1.3" }\n'+
    'authorization { users: [\n'+users.join(',\n')+'\n] }\n';
  if(routes.length){
    if(!cluster_listen||routes.length!==3)throw Error('The replicated profile requires exactly three route addresses');
    for(const route of routes){const u=new URL(route);if(u.protocol!=='nats-route:'||u.username||u.password||u.search||u.hash)throw Error('Invalid private broker route')}
    result+='cluster { name: '+q('infrapro-'+environment)+', listen: '+q(cluster_listen)+', routes: '+q(routes)+', tls: { ca_file: '+q(route_ca_file)+', cert_file: '+q(route_cert_file)+', key_file: '+q(route_key_file)+', verify_cert_and_check_known_urls: true, min_version: "1.3" } }\n';
  }
  return result;
}
if(resolve(process.argv[1]??'')===fileURLToPath(import.meta.url)){
  const [profile,epoch,out]=process.argv.slice(2);
  if(!profile||!epoch||!out)throw Error('Usage: node deploy/events/render.mjs <node-profile.json> <restore-epoch> <nats.conf>');
  mkdirSync(dirname(resolve(out)),{recursive:true,mode:0o700});
  writeFileSync(out,render(JSON.parse(readFileSync(profile,'utf8')),epoch),{mode:0o600});
}
