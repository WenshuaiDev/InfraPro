// Isolated synthetic cloud node. No business database credentials enter its config.
import {execFileSync,spawn} from 'node:child_process';
import {mkdirSync,writeFileSync,readFileSync,appendFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {randomUUID,X509Certificate} from 'node:crypto';
import https from 'node:https';
import {availablePort,stop} from './parsing-lab.mjs';
export async function startRelay(directory=resolve('.artifacts/ingestion/relay')){
 mkdirSync(directory,{recursive:true,mode:0o700});
 execFileSync('docker',['compose','-f','deploy/ingestion-lab/compose.yaml','up','-d','--wait'],{stdio:'inherit'});
 const schema='relay_'+randomUUID().replaceAll('-','');
 execFileSync('docker',['compose','-f','deploy/ingestion-lab/compose.yaml','exec','-T','postgres','psql','-U','relay_test','-d','relay_test','-c',`CREATE SCHEMA ${schema}`],{stdio:'pipe'});
 const openssl=args=>execFileSync('openssl',args,{cwd:directory,stdio:'pipe'});
 openssl(['req','-x509','-newkey','rsa:2048','-nodes','-keyout','ca.key','-out','ca.pem','-days','2','-subj','/CN=InfraPro synthetic ingestion CA']);
 for(const name of ['server','producer','consumer','control','outsider']){
  openssl(['req','-newkey','rsa:2048','-nodes','-keyout',`${name}.key`,'-out',`${name}.csr`,'-subj',`/CN=${name}`]);
  writeFileSync(join(directory,name+'.ext'),`subjectAltName=IP:127.0.0.1,DNS:localhost\nextendedKeyUsage=${name==='server'?'serverAuth':'clientAuth'}\n`);
  openssl(['x509','-req','-in',name+'.csr','-CA','ca.pem','-CAkey','ca.key','-CAcreateserial','-out',name+'.pem','-days','2','-extfile',name+'.ext']);
 }
 const id=randomUUID(),port=await availablePort(),ddp=await availablePort(),http=await availablePort();
 const principals={};for(const role of ['producer','consumer','control'])principals[new X509Certificate(readFileSync(join(directory,role+'.pem'))).fingerprint256.replaceAll(':','').toLowerCase()]={id:'lab-'+role,role,...(role==='consumer'?{consumer_id:'lab'}:{})};
 writeFileSync(join(directory,'principals.json'),JSON.stringify(principals),{mode:0o600});
 const cfg={relay_id:id,database_url:`postgres://relay_test:relay-test-local-only@127.0.0.1:56532/relay_test?sslmode=disable&search_path=${schema}`,control_address:`127.0.0.1:${port}`,certificate_file:join(directory,'server.pem'),key_file:join(directory,'server.key'),client_ca_file:join(directory,'ca.pem'),principals_file:join(directory,'principals.json'),budget_bytes:String(2**32),listener_port_min:1024,listener_port_max:65535,listen_addresses:['127.0.0.1'],max_connections:128};
 const config=join(directory,'relay.json'),binary=join(directory,'ingest-relay');writeFileSync(config,JSON.stringify(cfg,null,2),{mode:0o600});
 execFileSync('go',['build','-trimpath','-o',binary,'./services/ingest-relay/cmd/ingest-relay'],{stdio:'inherit'});
 execFileSync(binary,['migrate',config],{stdio:'inherit'});
 const files=name=>({certificate_file:join(directory,name+'.pem'),key_file:join(directory,name+'.key'),ca_file:join(directory,'ca.pem')});
 const worker={enabled:true,storage_budget_bytes:String(2**32),queue_budget:500000,relays:[{id,configuration_ref:'synthetic-lab',url:`https://127.0.0.1:${port}`,consumer_id:'lab',consumer:files('consumer'),control:files('control')}]};
 const workerConfig=join(directory,'worker.json');writeFileSync(workerConfig,JSON.stringify(worker,null,2),{mode:0o600});
 const tls={ca:readFileSync(join(directory,'ca.pem')),cert:readFileSync(join(directory,'consumer.pem')),key:readFileSync(join(directory,'consumer.key'))};
 let child;
 async function resume(){child=spawn(binary,['serve',config],{stdio:['ignore','pipe','pipe'],env:{...process.env,INFRAPRO_DIAGNOSTICS_INSTANCE_ID:'ingestion-lab',INFRAPRO_DIAGNOSTICS_NODE_ID:'local-cloud',INFRAPRO_DIAGNOSTICS_REPLICA_ID:'ingest-relay',INFRAPRO_DIAGNOSTICS_REPORT_FILE:join(directory,'report.json')}});for(const s of [child.stdout,child.stderr])s.on('data',v=>appendFileSync(join(directory,'service.log'),v));for(let n=0;n<100;n++){if(await new Promise(ok=>https.get(worker.relays[0].url+'/readyz',tls,res=>{res.resume();ok(res.statusCode===200);}).on('error',()=>ok(false))))return;await new Promise(r=>setTimeout(r,100));}throw Error('Cloud relay startup failed; inspect protected local files');}
 const producerFor=name=>{if(!/^[a-z0-9]+$/.test(name))throw Error('Invalid synthetic principal');const cert='producer-'+name;openssl(['req','-newkey','rsa:2048','-nodes','-keyout',cert+'.key','-out',cert+'.csr','-subj','/CN='+cert]);writeFileSync(join(directory,cert+'.ext'),'extendedKeyUsage=clientAuth\n');openssl(['x509','-req','-in',cert+'.csr','-CA','ca.pem','-CAkey','ca.key','-CAcreateserial','-out',cert+'.pem','-days','2','-extfile',cert+'.ext']);const principal='lab-'+name;principals[new X509Certificate(readFileSync(join(directory,cert+'.pem'))).fingerprint256.replaceAll(':','').toLowerCase()]={id:principal,role:'producer'};writeFileSync(join(directory,'principals.json'),JSON.stringify(principals),{mode:0o600});return {principal,...files(cert)};};
 const pause=async()=>{await stop(child);child=undefined;};await resume();
 return {id,directory,schema,config,workerConfig,producerFor,ddpPort:ddp,httpPort:http,producer:files('producer'),pause,resume,close:pause,env:{INFRAPRO_INGESTION_TEST_RELAY_CONFIG:config,INFRAPRO_INGESTION_FILE:workerConfig,INFRAPRO_INGESTION_TEST_DDP_PORT:String(ddp),INFRAPRO_INGESTION_TEST_HTTP_PORT:String(http),INFRAPRO_INGESTION_TEST_PRODUCER:join(directory,'producer'),INFRAPRO_INGESTION_TEST_CA:join(directory,'ca.pem')}};
}
