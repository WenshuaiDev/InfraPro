import { execFileSync } from 'node:child_process';
import { openSync,closeSync } from 'node:fs';
import { join } from 'node:path';
import { loadLab,cleanEnv } from './fixtures/diagnostics-lab.mjs';
const lab=loadLab();if(lab.faults.length)throw Error('Recover lab before real service checks');
const log=join(lab.directory,'evidence/real-services.log'),fd=openSync(log,'w',0o600);
try{execFileSync('go',['test','-v','-count=1','-timeout','3m','./services/diagnostics/internal/adapters','-run',process.argv.includes('--protocol-only')?'^TestRealFixedProtocols$':'^TestReal(FixedProtocols|DockerReadOnlyObservations)$'],{env:cleanEnv({INFRAPRO_DIAGNOSTICS_LAB_DIRECTORY:lab.directory}),stdio:['ignore',fd,fd]});console.log('Actual protocol and Docker checks passed; '+log)}catch{throw Error('Real service checks failed; private evidence: '+log)}finally{closeSync(fd)}
