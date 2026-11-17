import {build} from 'esbuild';
import {cp,mkdir,rm} from 'node:fs/promises';
await rm('dist',{recursive:true,force:true});await mkdir('dist');
await build({entryPoints:['src/app.ts'],outdir:'dist',bundle:true,format:'esm',platform:'browser',target:'es2022',minify:true,legalComments:'eof'});
await cp('index.html','dist/index.html');await cp('src/style.css','dist/style.css');
