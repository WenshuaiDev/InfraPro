import {startFilesLab} from './fixtures/files-lab.mjs';
import {seedFilesAcceptance} from './fixtures/files-acceptance.mjs';
const lab=await startFilesLab();try {const state=await seedFilesAcceptance(lab);console.log(`Open ${lab.origin}/t/${state.tenants[0]}/files. Synthetic credentials and samples: ${lab.directory}. Ctrl+C stops this isolated lab.`);await new Promise(r=>{process.once('SIGINT',r);process.once('SIGTERM',r);});}finally {await lab.close();}
