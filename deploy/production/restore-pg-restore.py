#!/usr/bin/python3
"""InfraPro PG17 restore adapter: target precreates public for btree_gist.
Only skips the archive SCHEMA - public entry. All other entries are unchanged.
The owning InfraPro restore command first requires an empty independent database.
"""
import os,pathlib,re,shutil,subprocess,sys,tempfile
os.umask(0o077)
real=str(pathlib.Path.home()/'env/infrapro/bin/pg_restore')
if '--list' in sys.argv or any(a.startswith('--use-list=') for a in sys.argv):
 os.execv(real,[real,*sys.argv[1:]])
with tempfile.TemporaryDirectory(prefix='infrapro-pg17-restore-') as td:
 d=pathlib.Path(td);archive=d/'control.dump';toc=d/'restore.list'
 with archive.open('wb') as f:shutil.copyfileobj(sys.stdin.buffer,f,1<<20)
 p=subprocess.run([real,'--list',str(archive)],capture_output=True)
 if p.returncode:sys.stderr.buffer.write(p.stderr);sys.exit(p.returncode)
 lines=p.stdout.decode().splitlines();kept=[];skipped=0
 for line in lines:
  if re.match(r'^\d+;\s+\d+\s+\d+\s+SCHEMA\s+-\s+public\s+',line):skipped+=1
  else:kept.append(line)
 if skipped>1:raise SystemExit('Unexpected duplicate public schema entries')
 toc.write_text('\n'.join(kept)+'\n')
 p=subprocess.run([real,*sys.argv[1:],'--use-list='+str(toc),str(archive)])
 sys.exit(p.returncode)
