import {execFileSync} from 'node:child_process';
import {mkdtempSync,mkdirSync,cpSync,writeFileSync,readFileSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {resolve,join} from 'node:path';
const root=resolve('.'),out=join(root,'.artifacts/display/packages');mkdirSync(out,{recursive:true});
for(const name of ['display-sdk','display-runtime','display-devtools'])execFileSync('pnpm',['--filter',`@infrapro/${name}`,'build'],{stdio:'inherit'});
const tarballs={};
for(const name of ['display-sdk','display-devtools']){const directory=join(root,'packages/ts',name),{version}=JSON.parse(readFileSync(join(directory,'package.json'),'utf8'));execFileSync('pnpm',['pack','--pack-destination',out],{cwd:directory,stdio:'inherit'});tarballs[`@infrapro/${name}`]=`file:${join(out,`infrapro-${name}-${version}.tgz`)}`;}
for(const template of ['legacy','monitoring-read','engineering-structure']){
  const directory=mkdtempSync(join(tmpdir(),`infrapro-display-${template}-`)),app=join(directory,'app');
  execFileSync('node',[join(root,'packages/ts/display-devtools/src/cli.mjs'),'init',app,...(template==='legacy'?[]:['--template',template])],{stdio:'inherit'});
  const p=JSON.parse(readFileSync(join(app,'package.json'),'utf8'));p.dependencies={...p.dependencies,...tarballs};p.devDependencies={...p.devDependencies,typescript:'5.9.3'};writeFileSync(join(app,'package.json'),JSON.stringify(p,null,2));
  execFileSync('pnpm',['install','--offline'],{cwd:app,stdio:'inherit'});execFileSync('pnpm',['build'],{cwd:app,stdio:'inherit'});
  for(const args of [['validate','dist','--manifest','display.manifest.json'],['pack','dist','--manifest','display.manifest.json','--out','display.zip'],['validate','display.zip']])execFileSync('pnpm',['exec','infrapro-display',...args],{cwd:app,stdio:'inherit'});
  const name=template==='legacy'?'example':`${template}-example`;cpSync(join(app,'display.zip'),join(root,`.artifacts/display/${name}.zip`));
  writeFileSync(join(root,`.artifacts/display/${template==='legacy'?'external':`${template}-external`}.json`),JSON.stringify({directory:app,standalone:true,packageDirectory:out,template},null,2));
}
console.log('Legacy, named-read and structure-model-2 external install/build/validate/pack passed.');
