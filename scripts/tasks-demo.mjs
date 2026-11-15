import { startTaskLab } from './fixtures/tasks-lab.mjs';
import { seedTaskAcceptance } from './fixtures/tasks-acceptance.mjs';
const lab=await startTaskLab();
try {await seedTaskAcceptance(lab);console.log(`Open ${lab.origin}/tasks. Synthetic credentials: ${lab.directory}/accounts.json. Ctrl+C stops this isolated lab.`);await new Promise(r=>{process.once('SIGINT',r);process.once('SIGTERM',r);});}finally{await lab.close();}
