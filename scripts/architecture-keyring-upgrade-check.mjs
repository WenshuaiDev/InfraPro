// The frozen ops image creates a mode-0600 keyring. Current ops and API images
// must be able to use that very same volume with read-only mounts.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { join,resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { read,root,write } from './architecture-lab.mjs';

export async function keyringUpgrade(directory){
  const label='arc-keyring-'+randomBytes(5).toString('hex'),volume=label+'-keys';
  const image=suffix=>label+':'+suffix;
  const env={...process.env,INFRAPRO_HTTP_TEST_HOST:'127.0.0.1'};
  const docker=(...args)=>execFileSync('docker',args,{cwd:root,env,encoding:'utf8',stdio:'pipe',maxBuffer:32<<20,timeout:600000}).trim();
  const recipe=read(join(directory,'baseline.json')),output=join(directory,'deploy','keyring-compatibility.json');
  const defaults=JSON.parse(docker('compose','--profile','ops','-f','deploy/compose/http-test.yaml','config','--format','json'));
  const profile=join(directory,'deploy','private',label,'compose.json');
  const services=Object.fromEntries(['postgres','redis','migrate','access-sync','api','ops'].map(role=>[role,defaults.services[role]]));
  for(const [name,s] of Object.entries(services)){
    if(name==='postgres'||name==='redis'){delete s.volumes;continue;}
    delete s.build;delete s.depends_on;s.image=image(name==='api'?'api':'ops');
    s.environment={...s.environment,INFRAPRO_HTTP_ADDR:'0.0.0.0:8080'};
    if(name==='api'||name==='ops')s.volumes=[{type:'volume',source:'keys',target:'/run/infrapro-mfa',read_only:true}];
  }
  services.api.ports=[{target:8080,host_ip:'127.0.0.1',published:'0'}];delete services.api.healthcheck;
  // Drop the upstream test project's fixed subnet as well as its Compose name.
  for(const s of Object.values(services))delete s.networks;
  write(profile,{name:label,services,volumes:{keys:{external:true,name:volume}}});
  const compose=(...args)=>docker('compose','-p',label,'-f',profile,...args);
  const mounted=mode=>'type=volume,source='+volume+',target=/run/infrapro-mfa'+(mode==='ro'?',readonly':'');
  const marker='set -eu; stat -c "%a:%u:%g" /run/infrapro-mfa/keyring.json; sha256sum /run/infrapro-mfa/keyring.json';
  let created=false;
  try{
    for(const [tag,src,file] of [['old',recipe.source,'infrapro'],['ops',root,'infrapro'],['api',root,'api']])
      docker('build','-t',image(tag),'-f',join(src,'deploy/docker/'+file+'.Dockerfile'),src);
    docker('volume','create','--label','infrapro.architecture.fixture='+label,volume);created=true;
    const provision=defaults.services['mfa-keys'].command[0].replaceAll('$$','$');
    docker('run','--rm','--user','0','--mount',mounted('rw'),'--entrypoint','/bin/sh',image('old'),'-ec',provision);
    const before=docker('run','--rm','--mount',mounted('ro'),'--entrypoint','/bin/sh',image('old'),'-ec',marker);
    assert.match(before,/^600:100:101\n[a-f0-9]{64}\s+\/run\/infrapro-mfa\/keyring.json$/);
    for(const tag of ['ops','api']){
      assert.equal(docker('image','inspect','-f','{{.Config.User}}',image(tag)),'100:101');
      assert.equal(docker('run','--rm','--mount',mounted('ro'),'--entrypoint','/bin/sh',image(tag),'-ec',marker),before);
    }
    compose('up','-d','--wait','postgres','redis');
    compose('run','--rm','--no-deps','migrate');compose('run','--rm','--no-deps','access-sync');
    compose('run','--rm','--no-deps','ops','account','mfa-keys');
    compose('up','-d','--no-deps','api');
    const address=compose('port','api','8080');assert.match(address,/^127\.0\.0\.1:\d+$/);
    let ready=false;
    for(let attempt=0;attempt<80&&!ready;attempt++){
      try{ready=(await fetch('http://'+address+'/readyz',{signal:AbortSignal.timeout(1500)})).ok;}
      catch{/* Wait for the real API to initialize its database, Redis and keyring. */}
      if(!ready)await new Promise(resolve=>setTimeout(resolve,200));
    }
    assert.ok(ready,'upgraded API must pass readiness using the old read-only volume');
    assert.equal(compose('exec','-T','api','/bin/sh','-ec',marker),before);
    const proof={status:'passed',baseline:recipe.commit,mode:'0600',uid:100,gid:101,readOnly:true,
      keyringSHA256:before.split('\n')[1].split(' ')[0],images:Object.fromEntries(['old','ops','api'].map(t=>[t,docker('image','inspect','-f','{{.Id}}',image(t))])),opsKeyStatus:true,apiReady:true};
    write(output,proof);return proof;
  }finally{
    compose('down','--volumes','--remove-orphans');
    if(created)docker('volume','rm',volume);
  }
}
if(process.argv[1]&&pathToFileURL(resolve(process.argv[1])).href===import.meta.url)console.log(await keyringUpgrade(resolve(process.argv[2])));
