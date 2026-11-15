import {existsSync,readFileSync,rmSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {startNotificationLab} from './fixtures/notification-lab.mjs';
import {seedNotificationAcceptance} from './fixtures/notifications-acceptance.mjs';
const args=process.argv.slice(2),directory=resolve('.artifacts/notifications/lab'),paused=resolve(directory,'worker.paused');
if(args.length){if(args.length!==1||!['--pause','--resume','--status'].includes(args[0]))throw Error('Usage: pnpm notifications:demo [--pause|--resume|--status]');const session=JSON.parse(readFileSync(resolve(directory,'session.json'),'utf8'));if(session.state!=='running')throw Error('No active synthetic notification lab');if(args[0]==='--pause')writeFileSync(paused,'paused');if(args[0]==='--resume')rmSync(paused,{force:true});console.log(JSON.stringify({...session,paused:existsSync(paused)},null,2));}else{
 const lab=await startNotificationLab();try{await seedNotificationAcceptance(lab);console.log(`Synthetic notification acceptance: ${lab.origin}/notifications\nAccounts: e2e_owner / notification_member\nPassword: Real browser 河流 secret 592\nPause/resume: pnpm notifications:demo --pause / --resume\nPress Ctrl+C to stop and remove only this lab's isolated data.`);await new Promise(resolve=>{process.once('SIGINT',resolve);process.once('SIGTERM',resolve);});}finally{await lab.close();}
}
