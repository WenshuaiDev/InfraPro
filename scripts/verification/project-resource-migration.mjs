// Reproducible v9 -> v10 drill using the approved pre-refactor source and an isolated test PostgreSQL database.
import {execFileSync} from 'node:child_process';
import {mkdtempSync,mkdirSync,readFileSync,writeFileSync,rmSync,chmodSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {resolve} from 'node:path';
import {randomBytes} from 'node:crypto';
import {testEnv,ensureTestDependencies} from '../test-environment.mjs';
const root=resolve(import.meta.dirname,'../..');const legacy=mkdtempSync(resolve(tmpdir(),'infrapro-v9-'));const evidence=resolve(root,'.artifacts/project-resource-migration');mkdirSync(evidence,{recursive:true,mode:0o700});chmodSync(evidence,0o700);
const database=`resource_upgrade_${randomBytes(8).toString('hex')}`;const testName=`migration_drill_${randomBytes(8).toString('hex')}_test.go`;const currentTest=resolve(root,'packages/go/platform/resources',testName);
const docker=args=>execFileSync('docker',['compose','-f',resolve(root,'deploy/compose/test.yaml'),'exec','-T','postgres',...args],{cwd:root});let created=false;
const env={...process.env,...testEnv,INFRAPRO_MIGRATION_EVIDENCE:evidence};
try {
 ensureTestDependencies();const archive=execFileSync('git',['archive','4e92f95'],{cwd:root,maxBuffer:64*1024*1024});execFileSync('tar',['-xf','-','-C',legacy],{input:archive});
 writeFileSync(resolve(legacy,'packages/go/platform/resources',testName),readFileSync(resolve(import.meta.dirname,'project-resource-legacy.go.txt')));
 execFileSync('go',['test','./packages/go/platform/resources','-run','^TestExportLegacyProjectResource$','-count=1','-v'],{cwd:legacy,env,stdio:'inherit'});
 docker(['createdb','-U','infrapro_test',database]);created=true;
 execFileSync('docker',['compose','-f',resolve(root,'deploy/compose/test.yaml'),'exec','-T','postgres','pg_restore','--exit-on-error','--no-owner','-U','infrapro_test','-d',database],{input:readFileSync(resolve(evidence,'legacy.dump')),stdio:['pipe','inherit','inherit']});
 const manifest=JSON.parse(readFileSync(resolve(evidence,'legacy.json')));const url=new URL(manifest.Config.DatabaseURL);url.pathname='/'+database;
 writeFileSync(currentTest,readFileSync(resolve(import.meta.dirname,'project-resource-upgrade.go.txt')));
 execFileSync('go',['test','./packages/go/platform/resources','-run','^TestUpgradeRestoredProjectResource$','-count=1','-v'],{cwd:root,env:{...env,INFRAPRO_MIGRATION_DATABASE:url.href},stdio:'inherit'});
 console.log('Verified backup restore, original login and resources, v1 receipt replay, v10 defaults. Evidence: .artifacts/project-resource-migration/report.json');
} finally {rmSync(currentTest,{force:true});rmSync(legacy,{recursive:true,force:true});if(created)docker(['dropdb','-U','infrapro_test',database]);}
