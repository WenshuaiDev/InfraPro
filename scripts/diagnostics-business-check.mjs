import { execFileSync } from 'node:child_process';
import { openSync, closeSync } from 'node:fs';
import { join } from 'node:path';
import { loadLab, readJSON, root, cleanEnv, command, ownedContainers, checkContainerOwnership } from './fixtures/diagnostics-lab.mjs';
const lab=loadLab();if(!lab.business||lab.faults.length)throw Error('Recover the complete owned business lab first');
const cfg=readJSON(lab.business_config),db=new URL(cfg.database_url);db.pathname='/diagnostics_lab_test';
const pg=ownedContainers(lab).find(id=>JSON.parse(command('docker',['inspect','--format','{{json .Config.Labels}}',id]))['io.infrapro.replica']==='postgres-1');checkContainerOwnership(lab,pg);
if(!command('docker',['exec',pg,'psql','-U','diagnostics_lab','-d','diagnostics_lab','-At','-c',"SELECT 1 FROM pg_database WHERE datname='diagnostics_lab_test'"]).trim())command('docker',['exec',pg,'createdb','-U','diagnostics_lab','diagnostics_lab_test']);
for(const c of readJSON(join(root,'contracts/diagnostics/runtime-catalog.json')).entries.filter(c=>c.entry_point?.startsWith('services/api/cmd/')))execFileSync('go',['build','-trimpath','-o',join(lab.directory,'bin',c.id),`./services/api/cmd/${c.id}`],{cwd:root,stdio:'inherit'});
const log=join(lab.directory,'evidence/owning-processes.log'),fd=openSync(log,'w',0o600);
try{execFileSync('go',['test','-count=1','-v','-timeout','8m','-tags','diagnosticsintegration monitoringintegration','./packages/go/platform/resources','-run','^TestDiagnosticsActualOwningProcesses$'],{cwd:root,env:cleanEnv({INFRAPRO_TEST_DATABASE_URL:db.toString(),INFRAPRO_TEST_REDIS_URL:cfg.redis_url,INFRAPRO_DIAGNOSTICS_LAB_DIRECTORY:lab.directory}),stdio:['ignore',fd,fd]});console.log(`Actual owning-process integration passed; evidence: ${log}`)}catch{throw Error(`Actual owning-process integration failed; private evidence: ${log}`)}finally{closeSync(fd)}
