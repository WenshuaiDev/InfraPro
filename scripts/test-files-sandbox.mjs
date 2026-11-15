import {execFileSync} from 'node:child_process';
import {mkdirSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
mkdirSync('.artifacts/files',{recursive:true});
const arch=execFileSync('docker',['info','--format','{{.Architecture}}'],{encoding:'utf8'}).trim().includes('arm')||execFileSync('docker',['info','--format','{{.Architecture}}'],{encoding:'utf8'}).trim()==='aarch64'?'arm64':'amd64';
execFileSync('go',['test','-c','-o','.artifacts/files/checker-linux.test','./packages/go/platform/files'],{env:{...process.env,GOOS:'linux',GOARCH:arch,CGO_ENABLED:'0'},stdio:'inherit'});
const result=execFileSync('docker',['run','--rm','--network','none','--read-only','--user','10001:10001','--memory','320m','--pids-limit','32','--cap-drop','ALL','--security-opt','no-new-privileges:true','--mount',`type=bind,src=${resolve('.artifacts/files/checker-linux.test')},dst=/checker.test,readonly`,'alpine:3.22','/checker.test','-test.run=TestCheckerLinuxKernelRestrictions','-test.v'],{encoding:'utf8'});
writeFileSync('.artifacts/files/linux-sandbox.log',result);process.stdout.write(result);
