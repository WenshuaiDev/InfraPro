import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { openSync, closeSync, readFileSync, existsSync } from 'node:fs';
import { dirname, resolve, join } from 'node:path';
import { loadLab } from './fixtures/diagnostics-lab.mjs';
import { evidence } from './fixtures/diagnostics-client.mjs';
const lab=loadLab();evidence(lab,'release',{result:'in_progress',command:'node scripts/diagnostics-delivery-check.mjs'});
const log=join(lab.directory,'evidence/release.log'),fd=openSync(log,'w',0o600);
const commands=[['pnpm',['build']],['pnpm',['check']],['node',['scripts/diagnostics-build.mjs','--skip-ui','--linux']],['docker',['build','-f','deploy/diagnostics/Dockerfile','-t','infrapro-diagnostics:local','.']]];
try{for(const[cmd,args]of commands){console.log('Release gate: '+cmd+' '+args.join(' '));execFileSync(cmd,args,{stdio:['ignore',fd,fd]})}}catch{throw Error('Release gate failed; private evidence: '+log)}finally{closeSync(fd)}
const native=JSON.parse(execFileSync('build/bin/infrapro-diagnostics',['version','--verify-assets'],{encoding:'utf8'})),container=JSON.parse(execFileSync('docker',['run','--rm','--network','none','--memory','64m','--label','io.infrapro.lab.run_id='+lab.run_id,'infrapro-diagnostics:local','version','--verify-assets'],{encoding:'utf8'}));assert(native.ui_ready&&container.ui_ready);assert.equal(native.build_id,container.build_id);
for(const path of ['docs/operations/independent-runtime-diagnostics.md','docs/development/diagnostic-process-reports.md','deploy/diagnostics/README.md']){const text=readFileSync(path,'utf8');for(const match of text.matchAll(/\]\(([^)]+)\)/g)){if(!/^(https?:|#)/.test(match[1]))assert(existsSync(resolve(dirname(path),match[1].split('#')[0])),'Missing document link: '+match[1])}}
evidence(lab,'release',{command:'node scripts/diagnostics-delivery-check.mjs',commands:commands.map(([cmd,args])=>cmd+' '+args.join(' ')),result:'passed',evidence_type:'root build/check + native/Linux release + standalone image',build_id:native.build_id,protocol_major:native.protocol_major,ui_ready:true,image:execFileSync('docker',['image','inspect','--format','{{.Id}}','infrapro-diagnostics:local'],{encoding:'utf8'}).trim(),recovery_result:'build and checks complete; no business runtime mutation; disposable version container removed'});console.log('Native and container release gates passed: '+native.build_id);
