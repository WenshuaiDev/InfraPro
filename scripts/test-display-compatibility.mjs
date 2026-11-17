import {execFileSync,spawn} from 'node:child_process';
import {mkdtempSync,mkdirSync,writeFileSync,createWriteStream,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {resolve,join} from 'node:path';
import {ensureTestDependencies,testEnv} from './test-environment.mjs';

// An immutable source archive is an input to this compatibility check. It does
// not create a branch or worktree and never contacts the user's application DB.
const baseline='9c744acfa26a3f754eb0710cd9052d5e5e8be9a7';
const out=resolve('.artifacts/display/monitoring-read');mkdirSync(out,{recursive:true});
const dir=mkdtempSync(join(tmpdir(),'infrapro-display-legacy-'));
let passed=false;
try {
  ensureTestDependencies();
  execFileSync('git',['archive','--format=tar',`--output=${join(dir,'source.tar')}`,baseline]);
  execFileSync('tar',['-xf',join(dir,'source.tar'),'-C',dir]);
  const command=join(dir,'tools/infrapro/cmd/display-readiness-probe');mkdirSync(command,{recursive:true});
  writeFileSync(join(command,'main.go'),`package main
import("context";"fmt";"os";"strings";"github.com/jackc/pgx/v5/pgxpool";"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations")
func main(){ctx:=context.Background();p,e:=pgxpool.New(ctx,os.Getenv("INFRAPRO_PROBE_DATABASE"));if e!=nil{os.Exit(2)};defer p.Close();e=migrations.Check(ctx,p);if e==nil||!strings.Contains(e.Error(),"migration"){os.Exit(3)};fmt.Printf("legacy binary rejected newer schema (compiled version %d)\\n",migrations.Version)}
`);
  const binary=join(dir,'probe');
  execFileSync('go',['build','-o',binary,'./tools/infrapro/cmd/display-readiness-probe'],{cwd:dir,stdio:'inherit'});
  await new Promise((resolveRun,reject)=>{
    const log=createWriteStream(join(out,'compatibility.log'));
    const child=spawn('go',['test','-race','./packages/go/platform/resources','-run','^TestDisplayReadMigrationFormatsAndDowngrade$','-count=1','-v','-timeout=3m'],{env:{...process.env,...testEnv,INFRAPRO_DISPLAY_LEGACY_PROBE:binary},stdio:['ignore','pipe','pipe']});
    for(const stream of [child.stdout,child.stderr])stream.on('data',chunk=>{log.write(chunk);process.stdout.write(chunk);});
    child.on('error',reject);child.on('close',code=>{log.end();if(code===0)resolveRun();else reject(Error('Compatibility test failed'));});
  });
  passed=true;
} finally {
  writeFileSync(join(out,'compatibility-result.json'),JSON.stringify({baseline,passed,completedAt:new Date().toISOString()},null,2));
  rmSync(dir,{recursive:true,force:true});
}
