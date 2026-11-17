import { execFileSync } from 'node:child_process';
import { openSync, closeSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import assert from 'node:assert/strict';
import { loadLab, cleanEnv, root } from './fixtures/diagnostics-lab.mjs';
import { evidence } from './fixtures/diagnostics-client.mjs';
import { checkCoverage } from './diagnostics-coverage.mjs';
const lab=loadLab(),coreOnly=process.argv.includes('--core');
if(lab.faults.length)throw Error('Recover diagnostics lab before verification');
const log=join(lab.directory,'evidence/core-go.jsonl'),fd=openSync(log,'w',0o600);
const args=['test','-json','-race','-count=1','-timeout','5m','-skip','^TestRealFixedProtocols$|^TestRealDockerReadOnlyObservations$|^TestPrivateDBusSystemdContract$|^TestLinuxContainerResourceCollection$','./services/diagnostics/...','./packages/go/foundation/diagnosticreport/...','./packages/go/foundation/securefile/...','./tools/infrapro/internal/command/...','./services/api/internal/transport/http/display/...'];
try{execFileSync('go',args,{cwd:root,env:cleanEnv(),stdio:['ignore',fd,fd]})}catch{throw Error('Diagnostics core checks failed: '+log)}finally{closeSync(fd)}
const events=readFileSync(log,'utf8').split('\n').filter(x=>x.startsWith('{')).map(x=>JSON.parse(x));
const cases=events.filter(e=>e.Action==='pass'&&e.Test).map(e=>({package:e.Package,test:e.Test,result:'passed',elapsed_seconds:e.Elapsed}));assert(cases.length>35);assert(!events.some(e=>e.Action==='skip'&&e.Test),'Unexpected skip in required core checks');
evidence(lab,'core-tests',{command:'go '+args.join(' '),result:'passed',evidence_type:'unit/contract/real-local-HTTP',go_version:execFileSync('go',['version'],{encoding:'utf8'}).trim(),cases,recovery_result:'Go test cleanup completed'});
const coverage=checkCoverage();assert.throws(()=>checkCoverage({additionalEntries:['services/api/cmd/unknown-diagnostics-fixture/main.go']}),/Missing runtime catalog ownership/);
const checks=[['scripts/diagnostics-contract.mjs','--check'],['scripts/diagnostics-templates.mjs','--check']];for(const args of checks)execFileSync('node',args,{cwd:root,stdio:'inherit'});
evidence(lab,'contracts',{command:'node scripts/diagnostics-contract.mjs --check; node scripts/diagnostics-templates.mjs --check; checkCoverage with unknown cmd negative fixture',result:'passed',evidence_type:'contract',coverage,unknown_cmd_rejected:true,generation_idempotent:true,recovery_result:'no configuration or business data changed'});
console.log('Diagnostics core and contract checks passed: '+cases.length+' cases');
if(!coreOnly){
 execFileSync('node',['scripts/diagnostics-build.mjs','--skip-ui','--linux'],{cwd:root,stdio:'inherit'});
 execFileSync('docker',['build','-f','deploy/diagnostics/Dockerfile','-t','infrapro-diagnostics:local','.'],{cwd:root,stdio:'inherit'});
 for(const file of ['diagnostics-native-check.mjs','diagnostics-business-check.mjs','diagnostics-regression-check.mjs','diagnostics-services-check.mjs','diagnostics-linux-check.mjs','diagnostics-network-check.mjs','diagnostics-fault-check.mjs','diagnostics-deploy-check.mjs','diagnostics-browser-check.mjs'])execFileSync('node',['scripts/'+file],{cwd:root,stdio:'inherit'});
}
