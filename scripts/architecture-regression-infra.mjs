import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { chmodSync, existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { availablePort } from './fixtures/parsing-lab.mjs';
import { digest, fileLease, read, sleep, write } from './architecture-lab.mjs';
import { installClockTrialSetup } from './architecture-trial-setup.mjs';
import { installAuditRoleSetup } from './architecture-audit-role-setup.mjs';
import { installDisplayBoundarySetup } from './architecture-display-boundary-setup.mjs';
import { ensureOwnedDockerNetwork, unusedDockerSubnet } from './architecture-http-network.mjs';

function ownedRegressionNetwork(project, group, manifest) {
  const prefixes = { identity:'10.248', files:'10.247', 'derived-database':'10.244' };
  if (!Object.hasOwn(prefixes,group)) return { default: {} };
  const existing = existsSync(manifest) ? read(manifest) : null;
  if (existing) assert.equal(existing.name, project);
  const legacy = group === 'derived-database' && existing &&
    JSON.stringify(existing.networks) === JSON.stringify({ default: {} });
  if (legacy) {
    const result = spawnSync('docker', ['network','inspect',project + '_default','--format',
      '{{index .Labels "com.docker.compose.project"}}'], {encoding:'utf8'});
    if (result.error) throw result.error;
    if (result.status === 0) {
      assert.equal(result.stdout.trim(),project,'Unexpected owner for the retained derived database network');
      return existing.networks;
    }
    assert(/not found|no such network/i.test(result.stderr),'Unable to check the retained test network');
    const active = execFileSync('docker',['ps','--filter','label=com.docker.compose.project='+project,'--format','{{.Names}}'],{encoding:'utf8'}).trim();
    assert.equal(active,'','An active original command is still using the derived database network');
  }
  const name = ensureOwnedDockerNetwork({
    name: project + '-network', owner: project, label: 'infrapro.architecture.' + group + '-regression',
    prefix: prefixes[group],
  });
  const networks = { default: { external: true, name } };
  if (legacy) write(manifest,{...existing,networks});
  return networks;
}

async function accountTestStorage(cwd,workspace,project,mapping,group){
  const manifest=join(workspace,'account-compose.json');
  const networks=ownedRegressionNetwork(project,group,manifest);
  if(!existsSync(manifest)){
    const original=JSON.parse(execFileSync('docker',['compose','-f',join(cwd,'deploy/compose/test.yaml'),'config','--format','json'],{encoding:'utf8'}));
    const used=new Set(Object.values(mapping));
    for(const service of Object.values(original.services))for(const binding of service.ports){
      const old=String(binding.published);
      let assigned;do{assigned=String(await availablePort());}while(used.has(assigned));
      used.add(assigned);mapping[old]=assigned;binding.published=assigned;
    }
    write(manifest,{name:project,services:original.services,networks});
  }else{
    const original=read(manifest);
    if(networks.default.external)assert.deepEqual(original.networks,networks);
    for(const [role,old] of [['postgres','55432'],['redis','56379']])mapping[old]=original.services[role].ports[0].published;
  }
  return manifest;
}

function regressionStorage(dir, side, group) {
  assert(['baseline','candidate'].includes(side));
  assert(['general','soak','monitoring','restore','https','derived-database','identity','files'].includes(group));
  const owner=group==='general'?side:side+'-'+group;
  const lab=read(join(dir,'private/lab.json'));
  return { project:'arc-reg-'+lab.id+'-'+owner, workspace:join(dir,'private/regression-storage',owner) };
}

// The caller holds this storage group's lease until both Compose projects stop.
// Keep the containers and their volumes available for the next original command.
export function stopRegressionStorage(dir, side, group = 'general') {
  const { project, workspace } = regressionStorage(dir, side, group);
  const manifests = [
    [project, join(workspace, 'compose.json')],
    [project + '-accounts', join(workspace, 'account-compose.json')],
  ].filter(([, manifest]) => existsSync(manifest));
  for (const [name, manifest] of manifests)
    assert.equal(read(manifest).name, name, 'Unexpected Compose project in the test storage manifest');
  const observedGroups = ['derived-database', 'monitoring', 'identity', 'files', 'restore'];
  const states = () => manifests.flatMap(([name]) => {
    const lines = execFileSync('docker', ['ps', '--all', '--filter',
      'label=com.docker.compose.project=' + name, '--format', '{{json .}}'], { encoding: 'utf8' }).trim();
    return lines ? lines.split('\n').map((line) => {
      const { ID, Names, State } = JSON.parse(line);
      return { id: ID, name: Names, state: State };
    }) : [];
  });
  const runningBeforeStop = ['files', 'restore'].includes(group) ? states().filter(c => c.state === 'running') : undefined;
  for (const [name, manifest] of manifests) {
    execFileSync('docker', ['compose', '-p', name, '-f', manifest, 'stop'], { stdio: 'pipe' });
  }
  if (observedGroups.includes(group)) {
    const containers = states();
    for (const item of containers) assert.equal(item.state, 'exited', 'Test component still running: ' + item.name);
    const at = new Date().toISOString();
    write(join(workspace, 'parking', at.replaceAll(':', '-') + '.json'), { at, side, project, runningBeforeStop, containers });
  }
}

function readGoOverlay(flags) {
  const path = flags?.match(/(?:^| )-overlay=(\S+)/)?.[1];
  return path ? Object.entries(read(path).Replace).map(([source, replacement]) =>
    [source, readFileSync(replacement, 'utf8')]) : [];
}

// The public HTTP recorder and Playwright both emit replacements for the same
// files. Copy their bytes before another invocation can regenerate the recorder.
export async function captureRecordingOverlay(lock, environment, prepare) {
  const unlock = await fileLease(lock);
  try {
    Object.assign(environment, await prepare());
    return readGoOverlay(environment.GOFLAGS);
  } finally {
    await unlock();
  }
}

export function mapGoTestEndpoints(source, text, mapping, project, accounts) {
  let adjusted = text.replace(/\b(127\.0\.0\.1|localhost|host\.docker\.internal):(\d+)\b/g,
    (address, host, port) => mapping[port] ? host + ':' + mapping[port] : address);
  if (source.includes('/monitoring/contracttest/') || source.endsWith('/internal/transport/http/monitoring_e2e_test.go')) {
    // The cleanup opens each replica from a bare port string.
    adjusted = adjusted.replace(/"(\d+)"/g, (quoted, port) => mapping[port] ? '"' + mapping[port] + '"' : quoted);
  }
  if (source.endsWith('/resources/monitoring_routes_test.go') || source.endsWith('/resources/monitoring_storage_test.go')) {
    const original = '[]string{"59000", "59001"}';
    const local = '[]string{"' + mapping['59000'] + '", "' + mapping['59001'] + '"}';
    assert(mapping['59000'] && mapping['59001'] && mapping['59000'] !== mapping['59001']);
    assert(adjusted.includes(original) || adjusted.includes(local), 'Expected ClickHouse replica pair in ' + source);
    adjusted = adjusted.replaceAll(original, local);
  }
  return adjusted.replaceAll('infrapro-monitoring-contract-', project + '-')
    .replaceAll('infrapro-account-tests-', accounts + '-');
}

// The original monitoring contract deliberately stops its ClickHouse and S3
// containers. Keep every original assertion and change just the test endpoints
// and the Compose project they control, including calls issued from Go tests.
export async function regressionInfra(dir,side,cwd,existing={},browserEnvironment,group='general',requiresClickHouse=false,suiteName='') {
  const { project, workspace } = regressionStorage(dir, side, group);
  mkdirSync(workspace,{recursive:true,mode:0o700});
  const info=join(workspace,'ports.json'),compose=join(workspace,'compose.json');
  const networks=ownedRegressionNetwork(project,group,compose);
  let mapping;
  if(existsSync(compose)&&existsSync(info)){
    mapping=read(info);
    if(networks.default.external)assert.deepEqual(read(compose).networks,networks);
  }
  else {
    const source=JSON.parse(execFileSync('docker',['compose','-f',join(cwd,'deploy/monitoring-contract/compose.yaml'),'config','--format','json'],{encoding:'utf8'}));
    const ports={},used=new Set(),volumes={};
    for(const [role,spec] of Object.entries(source.services)){
      for(const port of spec.ports||[]){
        const old=String(port.published);
        let value;do{value=await availablePort();}while(used.has(value));used.add(value);
        ports[old]=String(value);port.published=String(value);
      }
      if(role==='s3'&&group!=='monitoring')spec.command=spec.command.map(v=>v.startsWith('-volume.max=')?'-volume.max=256':v);
      if(['s3','backup-s3','audit-volume','keeper','ch1','ch2'].includes(role)){
        const target=['s3','backup-s3','audit-volume'].includes(role)?'/data':role==='keeper'?'/var/lib/clickhouse-keeper':'/var/lib/clickhouse';
        if(!(spec.volumes||[]).some(v=>v.target===target)){
          volumes[role]={};(spec.volumes??=[]).push({type:'volume',source:role,target});
        }
      }
    }
    assert(Object.hasOwn(ports,'58333')&&Object.hasOwn(ports,'59000')&&Object.hasOwn(ports,'56432'));
    write(compose,{name:project,services:source.services,networks,volumes});mapping=ports;write(info,mapping);
  }
  const accounts=project+'-accounts',accountCompose=await accountTestStorage(cwd,workspace,accounts,mapping,group);
  const election = '-master.electionTimeout=3s';
  if (group === 'monitoring') {
    // S-01 restarts one master and requires a real pre-outage object within 20s.
    // The image's default 10s election can take 20s before the filer even rejoins.
    const definition = read(compose);
    const store = definition.services.s3;
    assert.equal(definition.name, project);
    assert.equal(store.command[0], 'server');
    assert(!store.command.some(arg => arg.startsWith('-master.peers=')));
    assert(store.volumes.some(mount => mount.type === 'volume' && mount.target === '/data'));
    if (!store.command.includes(election)) {
      store.command = [...store.command.filter(arg => !arg.startsWith('-master.electionTimeout=')), election];
      write(compose, definition);
    }
  }
  const config=join(workspace,'tools.json'),bin=join(workspace,'bin');mkdirSync(bin,{recursive:true,mode:0o700});
  const docker=execFileSync('/usr/bin/which',['docker'],{encoding:'utf8'}).trim();assert(docker.startsWith('/'));
  const monitoringRestart = group === 'monitoring' ? join(workspace, 'outages', randomUUID() + '.json') : undefined;
  let restoreNetworkFile;
  if (['test:monitoring:restore', 'test:display:restore'].includes(suiteName)) {
    // The original target retains its fixed published ports and fresh volumes.
    // Only its bridge subnet is selected while holding the target's lease.
    restoreNetworkFile = join(workspace, 'target-network.json');
    const subnet = unusedDockerSubnet(docker, '10.245', Number.parseInt(digest(project).slice(0, 2), 16));
    write(restoreNetworkFile, { networks: { default: { ipam: { config: [{ subnet }] } } } });
  }
  write(config,{project,cwd,compose,accounts,accountCompose,docker,mapping,monitoringRestart,restoreNetworkFile});
  // The notification restore only opens the original account PostgreSQL and Redis.
  if (suiteName !== 'test:notifications:restore') {
    // The original display fixture uploads into a pre-existing contract-source
    // bucket. An empty per-release store must establish the same prerequisite.
    execFileSync(docker,['compose','-f',compose,'up','-d','--wait','s3','audit-volume'],{stdio:['ignore','ignore','pipe']});
    if (group === 'monitoring') {
      const actual = JSON.parse(execFileSync(docker, [
        'inspect', '--format', '{"image":{{json .Image}},"command":{{json .Config.Cmd}},"mounts":{{json .Mounts}}}',
        project + '-s3-1',
      ], { encoding: 'utf8' }));
      const volume = actual.mounts.find(mount => mount.Type === 'volume' && mount.Destination === '/data');
      assert(volume?.Name && actual.command.includes(election));
      write(join(workspace, 'monitoring-store.json'), {
        project, image: actual.image, election, volume: volume.Name,
        composeSHA256: digest(readFileSync(compose)), observedAt: new Date().toISOString(),
      });
    }
    const required=['contract-ch','contract-source'];
    let verified=false,initial;
    for(let attempt=0;attempt<40&&!verified;attempt++){
      try{
        const shell=command=>execFileSync(docker,['exec','-i',project+'-s3-1','weed','shell','-master=s3:9333'],{input:command+'\n',encoding:'utf8',stdio:['pipe','pipe','pipe']});
        const exists=(listing,name)=>listing.split('\n').some(line=>line.trim().split(/\s+/)[0]===name);
        const before=shell('s3.bucket.list');
        initial??=Object.fromEntries(required.map(name=>[name,exists(before,name)]));
        for(const name of required)if(!exists(before,name))shell('s3.bucket.create -name='+name);
        const listed=shell('s3.bucket.list');verified=required.every(name=>exists(listed,name));
      }catch{/* Wait for the disposable volume and master to elect a leader. */}
      if(!verified)await sleep(350);
    }
    assert(verified,'Required original test buckets are unavailable on the isolated object store');
    write(join(workspace,'s3-prerequisites.json'),{at:new Date().toISOString(),initial,available:required});
  }
  // The original pipeline and soak drivers start the account database and
  // expect both analytics replicas to be ready before measuring their input.
  if(browserEnvironment || requiresClickHouse)
    execFileSync(docker,['compose','-f',compose,'up','-d','--wait','ch1','ch2'],{stdio:['ignore','ignore','pipe']});
  let inheritedOverlay;
  if(browserEnvironment){
    const storage={s3:'http://127.0.0.1:'+mapping['58333'],ch:'127.0.0.1:'+mapping['59000'],ch2:'127.0.0.1:'+mapping['59001']};
    inheritedOverlay = await captureRecordingOverlay(join(dir, 'regression', 'locks', 'http-overlay-' + side + '.flock'),
      existing, () => browserEnvironment(storage));
  }
  const shim=join(bin,'docker');
  writeFileSync(shim,'#!/usr/bin/env node\n'+
    "import {spawnSync} from 'node:child_process';import {readFileSync} from 'node:fs';import {resolve} from 'node:path';\n"+
    "const c=JSON.parse(readFileSync(process.env.ARC_REGRESSION_INFRA));const args=process.argv.slice(2);let project;\n"+
    "const restore=args.findIndex((arg,i)=>args[i-1]==='-f'&&resolve(arg).endsWith('/deploy/monitoring-restore/compose.yaml'));\n"+
    "if(restore>=0&&c.restoreNetworkFile)args.splice(restore+1,0,'-f',c.restoreNetworkFile);\n"+
    "for(let i=0;i<args.length;i++){if(args[i]==='-f' && args[i+1]){const path=resolve(args[i+1]);if(path.endsWith('/deploy/monitoring-contract/compose.yaml')){args[i+1]=c.compose;project=c.project;}else if(path.endsWith('/deploy/compose/test.yaml')){args[i+1]=c.accountCompose;project=c.accounts;}}else if(args[i].startsWith('infrapro-monitoring-contract-'))args[i]=c.project+args[i].slice('infrapro-monitoring-contract'.length);else if(args[i].startsWith('infrapro-account-tests-'))args[i]=c.accounts+args[i].slice('infrapro-account-tests'.length);}\n"+
    "if(project&&args[0]==='compose'&&!args.includes('-p')&&!args.includes('--project-name'))args.splice(1,0,'-p',project);\n"+
    "if(c.monitoringRestart&&project===c.project&&args.at(-1)==='s3'&&['start','stop'].includes(args.at(-2))){const {monitoringCompose}=await import("+
    JSON.stringify(new URL('./architecture-monitoring-restart.mjs',import.meta.url).href)+
    ");process.exitCode=await monitoringCompose(c,args);}else{const r=spawnSync(c.docker,args,{stdio:'inherit'});process.exitCode=r.status??1;}\n",{mode:0o700});
  chmodSync(shim,0o700);
  const hook=join(workspace,'endpoints.mjs');
  const mapper="function map(source,c){for(const [old,port] of Object.entries(c.mapping))for(const host of ['127.0.0.1','localhost','host.docker.internal'])source=source.replaceAll(host+':'+old,host+':'+port);return source.replaceAll('infrapro-monitoring-contract-',c.project+'-').replaceAll('infrapro-account-tests-',c.accounts+'-');}\n";
  writeFileSync(hook,"import{registerHooks}from'node:module';import{readFileSync}from'node:fs';import{fileURLToPath}from'node:url';\n"+
    "const c=JSON.parse(readFileSync(process.env.ARC_REGRESSION_INFRA));\n"+mapper+
    "registerHooks({load(url,ctx,next){const v=next(url,ctx);if(url.startsWith('file:')&&v.format==='module'&&/\\.m?js$/.test(url)&&fileURLToPath(url).startsWith(c.cwd+'/scripts/')&&v.source!=null){return {...v,source:map(Buffer.from(v.source).toString(),c)};}return v;}});\n",{mode:0o600});
  const replacements={};const overlayDir=join(workspace,'go');mkdirSync(overlayDir,{recursive:true,mode:0o700});
  for(const sub of ['packages/go','services/api','tools/infrapro']){
    const from=join(cwd,sub);
    for(const file of readdirSync(from,{recursive:true,withFileTypes:true})){
      if(!file.isFile()||!file.name.endsWith('_test.go'))continue;
      const source=join(file.parentPath,file.name),text=readFileSync(source,'utf8');
      const adjusted = mapGoTestEndpoints(source, text, mapping, project, accounts);
      if(adjusted===text)continue;
      const target=join(overlayDir,source.slice(cwd.length+1));
      mkdirSync(dirname(target),{recursive:true,mode:0o700});writeFileSync(target,adjusted);replacements[source]=target;
    }
  }
  for(const [source, text] of inheritedOverlay ?? readGoOverlay(existing.GOFLAGS)) {
    const target = join(overlayDir, 'merged-' + digest(source).slice(0, 12) + '.go');
    writeFileSync(target, mapGoTestEndpoints(source, text, mapping, project, accounts), { mode: 0o600 });
    replacements[source] = target;
  }
  const trialSetup=suiteName==='test:audit:regression' ? installClockTrialSetup(cwd,overlayDir,replacements) : undefined;
  const auditRoles=suiteName==='test:audit:regression' ? installAuditRoleSetup(dir,side,cwd,overlayDir,replacements) : undefined;
  const boundary=suiteName==='test:display:load' ? installDisplayBoundarySetup(cwd,overlayDir,replacements,
    join(workspace,'membership-fixtures',new Date().toISOString().replaceAll(':','-')),existing.ARC_DISPLAY_BOUNDARY_PLANNER) : undefined;
  const overlay=join(workspace,'overlay.json');write(overlay,{Replace:replacements});
  return {...existing,...(trialSetup&&{ARC_TRIAL_SETUP_EVIDENCE:JSON.stringify(trialSetup)}),
    ...(auditRoles&&{ARC_AUDIT_ROLE_SETUP:JSON.stringify(auditRoles)}),
    ...(boundary&&{ARC_DISPLAY_BOUNDARY_SETUP:JSON.stringify(boundary),ARC_DISPLAY_BOUNDARY_TRACE:boundary.traces}),
    ARC_REGRESSION_INFRA:config,PATH:bin+':'+process.env.PATH,NODE_OPTIONS:((existing.NODE_OPTIONS||process.env.NODE_OPTIONS||'')+' --import='+pathToFileURL(hook).href).trim(),
    GOFLAGS:((existing.GOFLAGS||'').replace(/(?:^| )-overlay=\S+/,'')+' -overlay='+overlay).trim()};
}
