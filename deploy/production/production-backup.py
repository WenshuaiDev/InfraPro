#!/usr/bin/python3
"""Frozen joint checkpoint. Root/systemd only; application CLI keeps its own user."""
import datetime,fcntl,hashlib,json,os,pathlib,pwd,shutil,subprocess,sys,tarfile,time,traceback
from s3_archive import S3,file_hash
from checkpoint_roles import Checkpoint
os.umask(0o077);B=pathlib.Path('/home/uadmin/env/infrapro');cfg=json.loads((B/'config/app/maintenance.json').read_text());release=json.loads((B/'config/release.json').read_text())['release'];cli=B/'releases'/release/'bin/infrapro';uid=pwd.getpwnam('uadmin').pw_uid
lock=open(B/'run/production-backup.lock','w');fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
ident=datetime.datetime.now(datetime.timezone.utc).strftime('checkpoint-%Y%m%dT%H%M%SZ');D=B/'data/backups'/ident;D.mkdir(mode=0o700);os.chown(D,uid,uid);started=time.monotonic();reason='Scheduled consistent production checkpoint '+ident
local_checkpoint=Checkpoint(B,ident)
# One coordinator per installation. Each host resolves its own installed replicas.
nodes=[152,37,151,38]
phases=local_checkpoint.spec['freezeOrder']
prepared_nodes=[]
state={'id':ident,'release':release,'started_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'state':'incomplete'}
sshconf=B/'config/backup-access/ssh_config';audit_readonly=False;maintenance_owned=False;file_prepared=None;file_sealed=False

def run(args,output=None):
 r=subprocess.run([str(a) for a in args],stdout=output or subprocess.PIPE,stderr=subprocess.PIPE)
 if r.returncode:raise RuntimeError(str(args[0])+': '+r.stderr.decode(errors='replace')[-1600:])
 return None if output else r.stdout

def app(*args):
 out=run(['runuser','-u','uadmin','--','env','INFRAPRO_CONFIG='+str(B/'config/app/maintenance.json'),'SSL_CERT_FILE='+str(B/'config/tls/ca.pem'),'PATH='+str(B/'bin')+':/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin',cli,*args]);return json.loads(out) if out else {}

def ssh(node,command,output=None):return run(['runuser','-u','uadmin','--','ssh','-F',sshconf,'backup-'+str(node),command],output)
def save(name,v):p=D/(name+'.json');p.write_text(json.dumps(v,ensure_ascii=False,indent=2));os.chown(p,uid,uid)
def status(phase):state['phase']=phase;save('checkpoint',state);print(phase,flush=True)
def checkpoint(node,action,phase=None):
 if node==152:
  if action=='prepare':return local_checkpoint.prepare()
  if action=='frozen':return local_checkpoint.frozen()
  return local_checkpoint.change(phase,action=='start')
 return json.loads(ssh(node,'checkpoint-'+action+' '+ident+(' '+phase if phase else '')))
def resume():
 global audit_readonly,maintenance_owned
 if audit_readonly:app('audit','mode','--mode','ready','--reason',reason);audit_readonly=False
 state.setdefault('resume',{})
 for phase in reversed(phases):
  for node in prepared_nodes:
   outcome=checkpoint(node,'start',phase)
   state['resume'].setdefault(str(node),{})[phase]=outcome
   save('checkpoint',state)
 if maintenance_owned:(B/'run/maintenance').unlink(missing_ok=True);maintenance_owned=False
try:
 status('freezing');state['initial_audit_mode']=app('audit','status')['mode']
 if state['initial_audit_mode']!='ready':raise RuntimeError('Audit must finish its prior maintenance operation before a new joint checkpoint')
 with (B/'run/maintenance').open('x') as marker:marker.write(ident+'\n')
 maintenance_owned=True;state['writers']={}
 for node in nodes:
  state['writers'][str(node)]=checkpoint(node,'prepare');prepared_nodes.append(node);save('checkpoint',state)
 for phase in phases:
  for node in nodes:checkpoint(node,'stop',phase)
 for node in nodes:state['writers'][str(node)]=checkpoint(node,'frozen')
 save('writer-boundaries',state['writers']);save('checkpoint',state)
 save('display-files',app('display','backup-manifest'))
 # Both sides are stopped. Save authoritative per-receipt storage/ACK frontiers.
 ingestion=(B/'releases'/release/'bin/ingestion-operator')
 if any(entry['role'] in ('ingest-worker','ingest-custody-worker') for host in state['writers'].values() for entry in host['units'].values()):
  run(['runuser','-u','uadmin','--','env','INFRAPRO_CONFIG='+str(B/'config/app/maintenance.json'),'SSL_CERT_FILE='+str(B/'config/tls/ca.pem'),ingestion,'manifest',D/'inner-receipts.json'])
 relay=B/'releases'/release/'bin/event-relay'
 if relay.is_file():
  output=run(['runuser','-u','uadmin','--','env','INFRAPRO_CONFIG='+str(B/'config/app/maintenance.json'),'INFRAPRO_EVENTS_FILE='+str(B/'config/architecture/maintenance-events.json'),relay,'status'])
  save('event-watermarks',json.loads(output))
 hold=app('audit','backup','hold','--kind','backup','--object-id',ident,'--until',(datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(days=7)).isoformat(),'--reason',reason);save('audit-hold',hold)
 file_prepared=app('files','backup','prepare','--directory',str(D/'files'),'--snapshot-ref',ident,'--reason',reason,'--external-dump');save('files-prepare',file_prepared)
 app('audit','mode','--mode','read_only','--reason',reason);audit_readonly=True
 status('monitoring-database-native-and-source');monitor=app('monitoring','backup','--directory',str(D/'monitoring'),'--reason',reason);save('monitoring-result',monitor)
 shutil.copy2(D/'monitoring/control.dump',D/'files/database.dump');os.chown(D/'files/database.dump',uid,uid)
 status('objects-and-node-metadata');objects={}
 for name in ['source','files','audit','parsing']:
  if name=='source':endpoint,key,secret,bucket=cfg['monitoring_object_endpoint'],cfg['monitoring_object_access_key'],cfg['monitoring_object_secret_key'],cfg['monitoring_source_bucket']
  else:
   c=cfg[name];endpoint=c['object_endpoint'];key=c.get('access_key',c.get('object_access_key'));secret=c.get('secret_key',c.get('object_secret_key'));bucket=c['bucket']
  objects[name]=S3(endpoint,key,secret,B/'config/tls/ca.pem').archive(bucket,D/'objects'/name)
 state['objects']=objects
 for node in nodes[1:]:
  with (D/('node-'+str(node)+'.tar.gz')).open('wb') as f:ssh(node,'export '+ident,f)
 with tarfile.open(D/'node-152.tar.gz','w:gz',compresslevel=1) as tar:
  local_checkpoint.add_node_data(tar)
  for name in ['config','units','ops']:tar.add(B/name,arcname=name)
  if any(u['role']=='diagnostics' for u in state['writers']['152']['units'].values()):tar.add(B/'data/diagnostics',arcname='diagnostics-state')
  tar.add(B/'releases',arcname='releases')
 cursor='';pages=[]
 while True:
  args=['audit','backup','manifest','--snapshot-ref',ident]
  if cursor:args+=['--cursor',cursor]
  page=app(*args);pages.append(page);cursor=page.get('next_cursor','')
  if not cursor:break
 save('audit-manifest',pages)
 app('files','backup','copy','--id',file_prepared['id'],'--directory',str(D/'files'),'--reason',reason)
 # All writers remain stopped; unfreeze audit only for the file seal maintenance event.
 app('audit','mode','--mode','ready','--reason',reason);audit_readonly=False
 save('files-seal',app('files','backup','seal','--id',file_prepared['id'],'--directory',str(D/'files'),'--reason',reason));file_sealed=True
 state['frozen_seconds']=round(time.monotonic()-started,2);resume();status('sealing-and-transferring')
 manifest=[]
 for p in sorted(D.rglob('*')):
  if p.is_file() and p.name not in ['checkpoint.json','archive-manifest.json']:
   h=file_hash(p)
   manifest.append({'path':str(p.relative_to(D)),'bytes':p.stat().st_size,'sha256':h})
 save('archive-manifest',manifest);state['state']='complete';state['files']=len(manifest);save('checkpoint',state)
 archive=D.with_suffix('.tar.gz')
 with tarfile.open(archive,'w:gz',compresslevel=1) as tar:tar.add(D,arcname=ident)
 h=file_hash(archive)
 with archive.open('rb') as f:
  proc=subprocess.Popen(['runuser','-u','uadmin','--','ssh','-F',str(sshconf),'backup-38','receive '+ident],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
  sent=0;transfer_start=time.monotonic()
  while chunk:=f.read(1<<20):
   proc.stdin.write(chunk);sent+=len(chunk);delay=sent/(8<<20)-(time.monotonic()-transfer_start)
   if delay>0:time.sleep(delay)
  proc.stdin.close();proc.stdin=None;stdout,stderr=proc.communicate()
 if proc.returncode:raise RuntimeError('Repository transfer failed: '+stderr.decode(errors='replace')[-800:])
 receipt=json.loads(stdout)
 if receipt['sha256']!=h:raise RuntimeError('Remote archive SHA256 mismatch')
 state.update(repository=receipt,elapsed_seconds=round(time.monotonic()-started,2),completed_at=datetime.datetime.now(datetime.timezone.utc).isoformat());save('checkpoint',state)
 native=S3(cfg['monitoring_backup_endpoint'],cfg['monitoring_backup_access_key'],cfg['monitoring_backup_secret_key'],B/'config/tls/ca.pem')
 removed=0
 for line in (D/'monitoring/files.ndjson').read_text().splitlines():
  entry=json.loads(line)
  if entry['kind']=='native':native.request('DELETE',cfg['monitoring_backup_bucket']+'/'+entry['key']);removed+=1
 state['native_staging_objects_removed']=removed;save('checkpoint',state)
 (B/'data/backups/latest.json').write_text(json.dumps(state,indent=2));print(json.dumps(state),flush=True)
 # Retain two local complete points too. Do not delete failed evidence or unverified files.
 completed=sorted([p for p in (B/'data/backups').glob('checkpoint-*') if p.is_dir() and (p/'checkpoint.json').exists() and json.loads((p/'checkpoint.json').read_text()).get('repository',{}).get('archive_verified')])
 for old in completed[:-2]:shutil.rmtree(old);old.with_suffix('.tar.gz').unlink(missing_ok=True)
except Exception as e:
 state.update(state='failed',error=str(e));save('checkpoint',state);print(json.dumps({'id':ident,'state':'failed','phase':state.get('phase'),'error':str(e)}),flush=True)
 # Keep an incomplete file freeze intact for diagnosis. Restarting other services is safe;
 # ordinary attachment writes stay blocked until the exact backup is completed and sealed.
 try:resume()
 except Exception as recovery:print('Resume failed: '+str(recovery),flush=True)
 raise
