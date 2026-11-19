import assert from 'node:assert/strict';
import { cpSync,existsSync,mkdirSync,readFileSync,renameSync,symlinkSync } from 'node:fs';
import { dirname,join,resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { digest,prepareLab,read,root,runDirectory,write } from './architecture-lab.mjs';
import { httpStorage } from './architecture-http-storage.mjs';
import { startSide } from './architecture-serve.mjs';
import { sharedConsoleFor } from './architecture-shared-console.mjs';
import { frozenConsoleIssuer,visualOutputDirectory } from './architecture-visual-frontend.mjs';

function frozenRoutesInput(dir) {
  const baseline=read(join(dir,'baseline.json')),snapshot=read(join(dir,'snapshot/manifest.json'));
  return {baseline:baseline.commit,archive:baseline.source_archive_sha256,snapshot:snapshot.pg.seed,
    binaries:digest(JSON.stringify(baseline.binaries)),assets:digest(JSON.stringify(baseline.staticFiles))};
}

function sameFrozenInputs(reference, expected, selection) {
  if (!reference || reference.consoleSHA256!==selection?.console.sha256 ||
      JSON.stringify(reference.inputs)!==JSON.stringify(expected)) return false;
  if (!selection) return !reference.deliveryReleaseSHA256;
  const issuer=frozenConsoleIssuer(reference.directory,selection);
  return Boolean(issuer && issuer.deliveryReleaseSHA256===reference.deliveryReleaseSHA256);
}

function configureVisualSpatial(dir,sides) {
  const lab=read(join(dir,'private/lab.json')),base=read(join(dir,'baseline.json'));
  const lock='tools/spatial-converter/requirements.lock';
  assert.equal(digest(readFileSync(join(base.source,lock))),digest(readFileSync(join(root,lock))));
  const python=join(root,'.artifacts/spatial-venv/bin/python');
  assert(existsSync(python),'Prepare the spatial converter used by the published spatial API');
  for(const side of sides){
    const file=lab[side].config,config=read(file);
    const work=join(dir,'private',side,'visual-spatial');mkdirSync(work,{recursive:true,mode:0o700});
    Object.assign(config,{spatial_python:python,spatial_converter_script:join(side==='b'?root:base.source,'tools/spatial-converter/convert.py'),spatial_temp_directory:work});
    write(file,config);
  }
}

// Both endpoints in this environment run the immutable baseline. Keeping it
// independent permits a later candidate to be compared against the same real
// self comparison, browser harness and screenshot set.
export async function frozenVisualLab(output,release=output,{fixtures=false}={}) {
  const selection=sharedConsoleFor(output),evidence=visualOutputDirectory(output,selection);
  const label=fixtures?'frozen-fixture-self':'frozen-self';
  const file=join(evidence,'visual',label+'.json'),expected=frozenRoutesInput(output);
  let reference=existsSync(file)?read(file):null;
  if (!sameFrozenInputs(reference,expected,selection)) {
    if(reference)write(join(evidence,'visual/attempts',label+'-'+Date.now()+'.json'),reference);
    const existingFile=join(evidence,'visual',fixtures?'fixture-environment.json':'environment.json');
    const existing=existsSync(existingFile)?read(existingFile):undefined;
    const source=existing?.snapshot===expected.snapshot&&existing.baseline===expected.baseline&&
      (!selection||existing.releaseSHA256===selection.backendReleaseSHA256)
      ? {dir:existing.directory,release:existing.releaseDirectory||release} : await visualLab(output,release,{fixtures});
    const inputs=frozenRoutesInput(source.dir);
    assert.deepEqual(inputs,expected);
    reference={directory:source.dir,preparedWith:source.release,inputs,fixtures,consoleSHA256:selection?.console.sha256,
      deliveryReleaseSHA256:selection?.deliveryReleaseSHA256,at:new Date().toISOString()};
    write(file,reference);
  }
  assert.deepEqual(frozenRoutesInput(reference.directory),expected);
  if(fixtures)configureVisualSpatial(reference.directory,['a','self']);
  return {dir:reference.directory,async start(){
    // This frozen run can outlive the candidate that originally prepared
    // its stores. Resume exactly the retained Compose volumes for its data.
    const lab=read(join(reference.directory,'private/lab.json'));
    const current=join(evidence,'visual',fixtures?'fixture-environment.json':'environment.json');
    const owners=[reference.storageOwner,existsSync(current)&&read(current).storageOwner,reference.directory,output].filter(Boolean);
    const owner=owners.find(dir=>{
      const file=join(dir,'private/http-storage/endpoints.json');
      return existsSync(file)&&read(file).chContainer===lab.chContainer;
    });
    assert(owner,'Frozen visual storage ownership does not match the recorded ClickHouse endpoint');
    const storage=await httpStorage(owner,read(join(owner,'private/lab.json')).id);
    assert.equal(storage.chContainer,lab.chContainer);
    if(reference.storageOwner!==owner){reference={...reference,storageOwner:owner};write(file,reference);}
    const stacks=[];
    try {
      for(const side of ['a','self'])stacks.push(await startSide(reference.directory,side,{releaseDirectory:reference.preparedWith}));
      return {close:()=>Promise.all(stacks.map(stack=>stack.close()))};
    }catch(error){await Promise.all(stacks.map(stack=>stack.close()));throw error;}
  }};
}

export async function visualLab(output,release=output,{fixtures=false}={}){
  const selection=sharedConsoleFor(output),evidence=visualOutputDirectory(output,selection);
  const path=join(evidence,'visual',fixtures?'fixture-environment.json':'environment.json');
  const original=join(evidence,'visual','environment.json');
  const originalEnvironment=existsSync(original)?read(original):null;
  const previous=existsSync(path)?read(path):null;
  const build=read(join(release,'candidate.json')),current=build.inputs.sourceSHA256;
  const frozen=read(join(output,'snapshot/manifest.json')).pg.seed;
  const record=previous?.releaseSHA256===current&&previous.snapshot===frozen&&previous.consoleSHA256===selection?.console.sha256?previous:null;
  if(selection)assert.equal(selection.backendReleaseSHA256,current);
  if(previous&&!record){
    const folder=join(evidence,'visual');
    write(join(folder,(fixtures?'fixture-environment-':'environment-')+previous.releaseSHA256+'.json'),previous);
    // Identical frontend bytes alone cannot qualify a different backend or
    // data snapshot. Archive the evidence with its actual source environment.
    const archive=join(folder,'attempts','releases',previous.releaseSHA256+'-'+Date.now());
    const frozen=join(folder,fixtures?'frozen-fixture-self.json':'frozen-self.json');
    const reference=existsSync(frozen)?read(frozen):null;
    const keepFrozen=sameFrozenInputs(reference,frozenRoutesInput(output),selection);
    const kinds=fixtures?[...(keepFrozen?['feature-states/b','published-screens/b']:['feature-states','published-screens']),'navigation','live-topology']:
      keepFrozen?['paired/b']:['paired'];
    for(const kind of kinds){
      const old=join(folder,kind);if(!existsSync(old))continue;
      mkdirSync(dirname(join(archive,kind)),{recursive:true,mode:0o700});renameSync(old,join(archive,kind));
    }
  }
  const prefix=fixtures?'run-visual-fixtures-':'run-visual-';
  const dir=record?.directory||runDirectory(join(dirname(output),prefix+read(join(output,'private/lab.json')).id+'-'+current.slice(0,12)));
  const baseline=read(join(output,'baseline.json'));
  for(const [name,src] of [['baseline-source',baseline.source],['baseline-bin',baseline.bin]]){
    if(!existsSync(join(dir,name)))symlinkSync(src,join(dir,name),'dir');
  }
  const parent=read(join(output,'private/lab.json')),baselineSnapshot=read(join(output,'snapshot/manifest.json'));
  const priorStorage=join(output,'visual',fixtures?'fixture-environment.json':'environment.json');
  const retained=existsSync(priorStorage)?read(priorStorage):null;
  const storageOwner=previous?.storageOwner||originalEnvironment?.storageOwner||retained?.storageOwner||previous?.directory||originalEnvironment?.directory||dir;
  const storageID=existsSync(join(storageOwner,'private/lab.json'))?read(join(storageOwner,'private/lab.json')).id:parent.id;
  const storage=await httpStorage(storageOwner,storageID);
  const lab=await prepareLab(dir,{directory:output,storage});
  if(selection)write(join(dir,'shared-console.json'),selection);
  if(fixtures)configureVisualSpatial(dir,['a','self','b']);
  const resources=read(join(dir,'snapshot/manifest.json'));
  assert.equal(resources.pg.seed,baselineSnapshot.pg.seed,'The visual cohort must replay the exact frozen S0');
  assert.equal(resources.pg.relay_seed,baselineSnapshot.pg.relay_seed);
  cpSync(join(release,'candidate.json'),join(dir,'candidate.json'));
  cpSync(join(release,'static-files.json'),join(dir,'static-files.json'));
  mkdirSync(join(evidence,'visual'),{recursive:true,mode:0o700});
  write(path,{directory:dir,fixtures,storageOwner,consoleSHA256:selection?.console.sha256,releaseDirectory:release,releaseSHA256:build.inputs.sourceSHA256,baseline:baseline.commit,
    snapshot:resources.pg.seed,databases:['arc_a','arc_self','arc_b'],postgres:lab.container,physical:resources.physical,objects:resources.objects.length});
  return {dir,release,async start(selected=['a','self','b']){
    assert(selected.length>0&&new Set(selected).size===selected.length&&selected.every(side=>['a','self','b'].includes(side)));
    const sides=[];
    try{
      for(const side of selected)sides.push(await startSide(dir,side,{releaseDirectory:release}));
      return {close:()=>Promise.all(sides.map(v=>v.close()))};
    }catch(e){await Promise.all(sides.map(v=>v.close()));throw e;}
  }};
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const lab=await visualLab(runDirectory(process.argv[2]),runDirectory(process.argv[3]||process.argv[2]));
  console.log('Isolated visual cohort',lab.dir);
}
