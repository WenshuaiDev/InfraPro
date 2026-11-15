import { loadLab, stopLab, processAlive, ownedContainers } from './fixtures/diagnostics-lab.mjs';
import { injectFault, recoverLab, faultCases } from './fixtures/diagnostics-faults.mjs';
const args=process.argv.slice(2).filter(a=>a!=='--'),lab=loadLab();
switch(args[0]??'status'){
 case 'status':console.log(JSON.stringify({run_id:lab.run_id,state:lab.state,origin:lab.origin,faults:lab.faults,processes:Object.fromEntries(Object.keys(lab.processes).map(n=>[n,processAlive(lab,n)])),owned_containers:ownedContainers(lab).length,evidence_directory:lab.directory+'/evidence'},null,2));break;
 case 'fault':console.log(JSON.stringify(await injectFault(lab,args[1]),null,2));break;
 case 'recover':console.log(JSON.stringify(await recoverLab(lab),null,2));break;
 case 'down':if(lab.faults.length)await recoverLab(lab);await stopLab(lab);console.log(`Stopped only ${lab.run_id}; evidence retained at ${lab.directory}/evidence`);break;
 default:throw Error(`Use status, fault (${faultCases.join(', ')}), recover or down`);
}
