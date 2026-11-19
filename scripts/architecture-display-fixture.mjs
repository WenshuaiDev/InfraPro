import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { readFileSync, existsSync, mkdirSync, mkdtempSync } from 'node:fs';
import { join } from 'node:path';
import { read, root, write, launch, sleep } from './architecture-lab.mjs';
import { testEnv } from './test-environment.mjs';

// Go's overlay changes only the load test, preserving the selected release,
// service binaries, schema, data-size assertions and the published fixture.
export async function displayFixture(dir,side) {
  const base=read(join(dir,'baseline.json')),lab=read(join(dir,'private/lab.json'));
  const source=side==='b'?root:base.source;
  const parent=join(dir,'private','display-load');
  mkdirSync(parent,{recursive:true,mode:0o700});
  const workspace=mkdtempSync(join(parent,side+'-')), secret=join(workspace,'fixture.json');
  const old=join(base.source,'packages/go/platform/resources/display_read_load_test.go');
  const target=join(source,'packages/go/platform/resources/display_read_load_test.go');
  let body=readFileSync(old,'utf8');
  for(const [before,after] of [
    ['f.publish(t, f.validate(t, f.uploadBytes(t, "load.csv"', 'original := f.publish(t, f.validate(t, f.uploadBytes(t, "load.csv"'],
    ['// The reference backend runs','if os.Getenv("ARC_DISPLAY_FIXTURE")!="" {architectureLoadFixture(t,f,p,screens,original,channels);return}\n\t// The reference backend runs'],
    ['15 * time.Minute','3 * time.Hour'],
  ]){assert(body.includes(before),'Frozen load fixture changed');body=body.replaceAll(before,after);}
  body+='\n'+readFileSync(join(root,'scripts/fixtures/architecture-display-load.txt'),'utf8');
  const overlay=join(workspace,'overlay.json'), edited=join(workspace,'display_read_load_test.go');
  const {writeFileSync}=await import('node:fs');
  writeFileSync(edited,body,{mode:0o600});
  write(overlay,{Replace:{[target]:edited}});
  const cfg=read(lab[side].config);
  const db=new URL(cfg.database_url);db.pathname='/arc_display_test';
  const exists=execFileSync('docker',['exec',lab.container,'psql','-U','archtest','-d','arc_seed','-Atc',"SELECT count(*) FROM pg_database WHERE datname='arc_display_test'"],{encoding:'utf8'}).trim();
  if(exists==='0')execFileSync('docker',['exec',lab.container,'createdb','-U','archtest','arc_display_test']);
  const args=['test','-overlay',overlay,'-tags','monitoringintegration','-timeout','100m','-count=1','-run','^TestDisplayReadLoadTenInstances$','-v','./packages/go/platform/resources'];
  const env={...process.env,...testEnv,INFRAPRO_TEST_DATABASE_URL:db.toString(),INFRAPRO_MONITORING_CONTRACT:'1',INFRAPRO_DISPLAY_READ_LOAD:'1',ARC_DISPLAY_FIXTURE:secret};
  const log=join(dir,'load','display',side+'-fixture.log');
  const child=launch('go',args,env,log,source);
  const finished=new Promise((ok,fail)=>{child.once('exit',code=>ok(code));child.once('error',fail);});
  try {
    const until=Date.now()+30*60*1000;
    while(!existsSync(secret) && Date.now()<until){
      if(child.exitCode!==null)throw Error(side+' display fixture failed with code '+child.exitCode+', see '+log);
      await sleep(250);
    }
    if(!existsSync(secret))throw Error(side+' display fixture setup timed out');
    return {workspace,secret,fixture:read(secret),log,async close(){write(secret+'.done',{finished:true});assert.equal(await finished,0);}};
  }catch(e){
    if(child.exitCode===null){write(secret+'.done',{aborted:true});child.kill('SIGTERM');}
    throw e;
  }
}
