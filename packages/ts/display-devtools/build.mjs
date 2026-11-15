import {cp,mkdir,rm} from 'node:fs/promises';
await rm('lib',{recursive:true,force:true});await mkdir('lib',{recursive:true});await cp('../../../apps/display-runtime/dist','lib/shell',{recursive:true});await cp('../display-sdk/dist','lib/sdk',{recursive:true});
