import {execFileSync} from 'node:child_process';
import {mkdirSync,openSync,closeSync,readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {ensureTestDependencies,testEnv} from './test-environment.mjs';
ensureTestDependencies();
execFileSync('docker',['compose','-f','deploy/monitoring-contract/compose.yaml','up','-d','--wait'],{stdio:'inherit'});
mkdirSync('.artifacts/files',{recursive:true});
execFileSync('go',['build','-o','.artifacts/files/files-checker','./services/api/cmd/files-checker'],{stdio:'inherit'});
const restore=process.argv.includes('--restore'),http=process.argv.includes('--http');
const env={...process.env,...testEnv,INFRAPRO_FILES_CONTRACT:'1',INFRAPRO_FILES_CHECKER_EXECUTABLE:resolve('.artifacts/files/files-checker'),INFRAPRO_FILES_RESTORE:restore?'1':'0'};
const logPath=`.artifacts/files/${restore?'restore':http?'http':'domain'}.log`,fd=openSync(logPath,'w');
try {execFileSync('go',['test','-race','-count=1','-timeout=10m','-v',http?'./services/api/internal/transport/http':'./packages/go/platform/resources','-run',restore?'^TestFilesFrozenBackup':'^TestFiles'],{env,stdio:['ignore',fd,fd]});} finally {closeSync(fd);process.stdout.write(readFileSync(logPath,'utf8'));}
if(!restore&&!http)execFileSync('go',['test','-race','./packages/go/platform/files','./packages/go/platform/internal/contentcheck','./packages/go/platform/config','./packages/go/platform/taskrecord'],{env,stdio:'inherit'});
