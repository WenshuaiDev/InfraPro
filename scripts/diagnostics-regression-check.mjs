import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { openSync,closeSync,readFileSync } from 'node:fs';
import { join } from 'node:path';
import { loadLab,readJSON,cleanEnv,ownedContainers,checkContainerOwnership,command } from './fixtures/diagnostics-lab.mjs';
import { evidence } from './fixtures/diagnostics-client.mjs';
const lab=loadLab();if(lab.faults.length)throw Error('Recover lab before business regression');
const cfg=readJSON(lab.business_config),db=new URL(cfg.database_url);db.pathname='/diagnostics_lab_test';
const pg=ownedContainers(lab).find(id=>JSON.parse(command('docker',['inspect','--format','{{json .Config.Labels}}',id]))['io.infrapro.replica']==='postgres-1');checkContainerOwnership(lab,pg);
if(!command('docker',['exec',pg,'psql','-U','diagnostics_lab','-d','diagnostics_lab','-At','-c',"SELECT 1 FROM pg_database WHERE datname='diagnostics_lab_test'"]).trim())command('docker',['exec',pg,'createdb','-U','diagnostics_lab','diagnostics_lab_test']);
const tests=['TestDisplayLifecycleAndSubjectBoundary','TestDisplayPackageWorkerLeastPrivilege','TestFilesUploadFactsAndImmutableAttachment','TestTaskRecordAtomicHistoryAndOutOfOrder','TestTaskCenterDisplayRealLifecycleAndIndexIndependence','TestTaskIndexRestrictedRole','TestAuditRealDisplayHandoffExhaustionAndMaintenanceReplay'];
const log=join(lab.directory,'evidence/business-regression.jsonl'),fd=openSync(log,'w',0o600),args=['test','-json','-race','-count=1','-timeout','6m','-tags','monitoringintegration','./packages/go/platform/resources','-run','^('+tests.join('|')+')$'];
try{execFileSync('go',args,{env:cleanEnv({INFRAPRO_TEST_DATABASE_URL:db.toString(),INFRAPRO_TEST_REDIS_URL:cfg.redis_url}),stdio:['ignore',fd,fd]})}catch{throw Error('Business regression failed: '+log)}finally{closeSync(fd)}
const events=readFileSync(log,'utf8').split('\n').filter(x=>x.startsWith('{')).map(x=>JSON.parse(x)),passed=events.filter(e=>e.Action==='pass'&&e.Test).map(e=>e.Test);for(const name of tests)assert(passed.includes(name),'Required business regression not passed: '+name);assert(!events.some(e=>e.Action==='skip'));
evidence(lab,'business-regression',{command:'go '+args.join(' '),result:'passed',evidence_type:'real-container PostgreSQL/Redis; private schema; actual business services',cases:tests.map(test=>({case:test,result:'passed'})),recovery_result:'private test schemas cleaned'});
const sandbox=join(lab.directory,'evidence/checker-sandbox.log'),sfd=openSync(sandbox,'w',0o600);try{execFileSync('node',['scripts/test-files-sandbox.mjs'],{stdio:['ignore',sfd,sfd]})}catch{throw Error('Checker sandbox regression failed: '+sandbox)}finally{closeSync(sfd)}
const output=readFileSync(sandbox,'utf8');assert(!output.includes('--- SKIP:'));assert(output.includes('PASS'));
evidence(lab,'checker-sandbox',{command:'node scripts/test-files-sandbox.mjs',result:'passed',evidence_type:'real Linux container/parent/child sandbox',recovery_result:'script-owned disposable containers removed'});console.log('Business lifecycle, permission, audit and Linux checker regressions passed');
