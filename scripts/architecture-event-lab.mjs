import { execFileSync } from 'node:child_process';
import { mkdirSync, readdirSync, readFileSync, writeFileSync, chmodSync, rmSync } from 'node:fs';
import { createServer } from 'node:net';
import { resolve, join } from 'node:path';
import { render } from '../deploy/events/render.mjs';

const [destination,environment,epoch,portInput,monitorInput]=process.argv.slice(2);
const certificateDays=process.env.INFRAPRO_EVENT_LAB_CERT_DAYS||'7';
if(!/^(?:[1-9]|[1-8][0-9]|90)$/.test(certificateDays))throw Error('Expected a certificate lifetime between 1 and 90 days');
if(!destination||!epoch||!environment)throw Error('Usage: node scripts/architecture-event-lab.mjs <empty-directory> <environment> <restore-epoch> [client-port] [monitor-port]');
const dir=resolve(destination);
mkdirSync(dir,{mode:0o700,recursive:true});
if(readdirSync(dir).length)throw Error('Use a fresh private event lab directory');
chmodSync(dir,0o700);
async function reserve(input){
  const server=createServer();
  const port=input===undefined?0:Number(input);
  if(!Number.isInteger(port)||port<0||port>65535)throw Error('Invalid local broker port');
  await new Promise((accept,reject)=>{server.once('error',reject);server.listen(port,'127.0.0.1',accept)});
  return server;
}
const client=await reserve(portInput),monitor=await reserve(monitorInput);
const port=client.address().port,monitorPort=monitor.address().port;
await Promise.all([client,monitor].map(s=>new Promise(resolve=>s.close(resolve))));
const ssl=(...args)=>execFileSync('openssl',args,{stdio:'pipe'});
const caKey=join(dir,'ca.key'),ca=join(dir,'ca.pem');
ssl('req','-new','-x509','-newkey','ec','-pkeyopt','ec_paramgen_curve:P-256','-nodes','-days',certificateDays,'-subj','/CN=InfraPro isolated event CA','-keyout',caKey,'-out',ca);
chmodSync(caKey,0o600);
const write=(file,body)=>writeFileSync(file,body,{mode:0o600});
const json=(file,value)=>write(file,JSON.stringify(value,null,2)+'\n');
const example=JSON.parse(readFileSync('deploy/events/events.example.json','utf8'));
function issue(role,server=false){
 const path=join(dir,role);
 mkdirSync(path,{mode:0o700,recursive:true});
 const csr=join(path,'req.pem'),key=join(path,'client.key'),cert=join(path,'client.pem'),ext=join(path,'req.ext');
 ssl('req','-new','-newkey','ec','-pkeyopt','ec_paramgen_curve:P-256','-nodes','-subj','/CN='+role+'.infrapro.internal','-keyout',key,'-out',csr);
 chmodSync(key,0o600);
 write(ext,'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage='+(server?'serverAuth':'clientAuth')+'\nsubjectAltName='+ (server?'IP:127.0.0.1,DNS:localhost':'DNS:'+role+'.infrapro.internal')+'\n');
 ssl('x509','-req','-in',csr,'-CA',ca,'-CAkey',caKey,'-CAcreateserial','-days',certificateDays,'-extfile',ext,'-out',cert);rmSync(csr);rmSync(ext);
 if(!server)json(join(path,'events.json'),{...example,environment,nats_url:'tls://127.0.0.1:'+port,replicas:1,ca_file:ca,cert_file:cert,key_file:key});
 return {cert,key};
}
const broker=issue('broker',true);
for(const role of ['event-admin','event-relay','parsing-dispatch-worker','monitoring-incremental-worker','monitoring-read-worker','task-index-worker','notification-worker','audit-worker'])issue(role);
mkdirSync(join(dir,'pg'),{mode:0o700});
json(join(dir,'pg/events.json'),{...example,mode:'pg',environment,replicas:1,nats_url:'',ca_file:'',cert_file:'',key_file:''});
const profile={environment,name:'lab1',listen:'127.0.0.1:'+port,monitor_port:monitorPort,ca_file:ca,cert_file:broker.cert,key_file:broker.key,store_dir:join(dir,'store'),max_file_bytes:4*1024**3};
json(join(dir,'profile.json'),profile);
write(join(dir,'broker.conf'),render(profile,epoch));
json(join(dir,'lab.json'),{environment,epoch,port,monitorPort,config:join(dir,'broker.conf'),role_files:'<role>/events.json'});
console.log(join(dir,'lab.json'));
