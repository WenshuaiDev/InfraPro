import {cp,mkdir,rm} from 'node:fs/promises';
await rm('lib',{recursive:true,force:true});await mkdir('lib',{recursive:true});await cp('../../../apps/display-runtime/dist','lib/shell',{recursive:true});await cp('../display-sdk/dist','lib/sdk',{recursive:true});

await cp('../../../examples/display-monitoring-read','lib/templates-read',{recursive:true,filter:p=>!/(^|\/)(node_modules|dist)(\/|$)/.test(p)&&!p.endsWith('.zip')});
await cp('../../../examples/display-engineering-structure','lib/templates-structure',{recursive:true,filter:p=>!/(^|\/)(node_modules|dist)(\/|$)/.test(p)&&!p.endsWith('.zip')});
