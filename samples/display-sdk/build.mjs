import {cp,mkdir,writeFile} from 'node:fs/promises';
await mkdir('dist',{recursive:true});await cp('index.html','dist/index.html');await cp('node_modules/@infrapro/display-sdk/dist','dist/sdk',{recursive:true,filter:p=>!p.endsWith('.d.ts')&&!p.endsWith('.map')});

await writeFile('dist/probe.wasm',new Uint8Array([0,97,115,109,1,0,0,0]));
