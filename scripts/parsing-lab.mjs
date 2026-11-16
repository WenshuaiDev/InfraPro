import {startParsingLab} from './fixtures/parsing-lab.mjs';
import {seedParsingAcceptance} from './fixtures/parsing-acceptance.mjs';
const lab=await startParsingLab({build:process.env.INFRAPRO_PARSING_SKIP_BUILD!=='1'});try{await seedParsingAcceptance(lab);console.log('Isolated parsing acceptance: '+lab.origin+' (accounts: '+lab.directory+'/accounts.json). Ctrl-C stops and removes only the owned test schema/bucket.');await new Promise(resolve=>{process.once('SIGINT',resolve);process.once('SIGTERM',resolve);});}finally{await lab.close();}
