#!/usr/bin/python3
"""Dedicated SSH forced command. No arbitrary paths, shells or commands."""
import datetime,hashlib,json,os,pathlib,re,shutil,subprocess,sys,tarfile,tempfile
from checkpoint_roles import Checkpoint
os.umask(0o077);os.nice(10);subprocess.run(['ionice','-c','3','-p',str(os.getpid())],check=True);base=pathlib.Path.home()/'env/infrapro';host=json.loads((base/'config/environment.json').read_text())['ip'];parts=os.environ.get('SSH_ORIGINAL_COMMAND','').split();action=parts[0] if parts else '';ident=parts[1] if len(parts)==2 else ''
if len(parts) not in (2,3) or not re.fullmatch(r'checkpoint-\d{8}T\d{6}Z',parts[1]):raise SystemExit('Invalid checkpoint command')
ident=parts[1]
if action.startswith('checkpoint-'):
 checkpoint=Checkpoint(base,ident)
 if action=='checkpoint-prepare' and len(parts)==2:result=checkpoint.prepare()
 elif action in ('checkpoint-frozen','checkpoint-business-frozen') and len(parts)==2:result=checkpoint.frozen(allow_brokers=action=='checkpoint-business-frozen')
 elif action in ('checkpoint-stop','checkpoint-start') and len(parts)==3:result=checkpoint.change(parts[2],action=='checkpoint-start')
 else:raise SystemExit('Invalid checkpoint operation')
 print(json.dumps(result));sys.exit()
if len(parts)!=2:raise SystemExit('Invalid checkpoint command')
if action=='export':
 with tempfile.TemporaryDirectory(dir=base/'data/backups',prefix='export-') as td:
  d=pathlib.Path(td)
  checkpoint=Checkpoint(base,ident);units=checkpoint.frozen()['units']
  if any(u['role']=='ingest-relay' for u in units.values()):
   with (d/'relay.dump').open('wb') as f:subprocess.run([str(base/'bin/pg_dump'),'-h',str(base/'run/pg'),'-p','55432','-d','infrapro_relay','-Fc','--no-owner','--no-acl'],stdout=f,check=True)
   release=json.loads((base/'config/release.json').read_text())['release']
   with (d/'relay-manifest.json').open('wb') as f:subprocess.run([str(base/'releases'/release/'bin/ingest-relay'),'manifest',str(base/'config/ingestion/relay.json')],stdout=f,check=True,env={**os.environ,'SSL_CERT_FILE':str(base/'config/tls/ca.pem')})
  with tarfile.open(fileobj=sys.stdout.buffer,mode='w|gz',compresslevel=1) as tar:
   checkpoint.add_node_data(tar)
   for name in ['config','units','ops']:tar.add(base/name,arcname=name)
   if any(u['role']=='diagnostics' for u in units.values()):tar.add(base/'data/diagnostics',arcname='diagnostics-state')
   if any(u['role']=='parser' for u in units.values()):tar.add(base/'releases',arcname='releases')
   for p in d.iterdir():tar.add(p,arcname=p.name)

 sys.exit()
if host!='10.23.56.38':raise SystemExit('Not a repository host')
repo=base/'data/checkpoints';repo.mkdir(mode=0o700,exist_ok=True);path=repo/(ident+'.tar.gz');receipt=repo/(ident+'.json')
if action=='receive':
 if path.exists() or receipt.exists():raise SystemExit('Checkpoint already exists')
 tmp=repo/(ident+'.incoming');h=hashlib.sha256();n=0;budget=300<<30
 try:
  with tmp.open('xb') as f:
   while chunk:=sys.stdin.buffer.read(1<<20):
    n+=len(chunk)
    if n>budget:raise RuntimeError('Checkpoint exceeds repository budget')
    if n%(64<<20)<(1<<20):
     disk=shutil.disk_usage(repo)
     if disk.free<disk.total*.2:raise RuntimeError('Repository must retain 20 percent free space')
    f.write(chunk);h.update(chunk)
   f.flush();os.fsync(f.fileno())
  total=sum(p.stat().st_size for p in repo.glob('*.tar.gz'))+n
  if total>budget:raise RuntimeError('Repository budget exceeded; old verified checkpoints retained')
  # Parse the entire archive to detect truncation before sealing.
  with tarfile.open(tmp,'r:gz') as tar:
   for member in tar:
    if member.isfile():
     with tar.extractfile(member) as f:
      while f.read(1<<20):pass
  tmp.rename(path);result={'id':ident,'bytes':n,'sha256':h.hexdigest(),'received_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'archive_verified':True};receipt.write_text(json.dumps(result,indent=2));print(json.dumps(result))
  complete=sorted([p for p in repo.glob('checkpoint-*.tar.gz') if p.with_suffix('').with_suffix('.json').exists()])
  for old in complete[:-2]:old.unlink();old.with_suffix('').with_suffix('.json').unlink()
 finally:
  if tmp.exists():tmp.unlink()
elif action=='fetch':
 if not receipt.exists():raise SystemExit('No verified checkpoint')
 with path.open('rb') as f:shutil.copyfileobj(f,sys.stdout.buffer,1<<20)
elif action=='status':print(receipt.read_text())
else:raise SystemExit('Unsupported backup command')
