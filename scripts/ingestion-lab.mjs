import {startIngestionLab,seedIngestionAcceptance} from './fixtures/ingestion-lab.mjs';
import {simulate} from './ingestion-simulate.mjs';
import {join} from 'node:path';
const lab=await startIngestionLab({build:process.env.INFRAPRO_INGESTION_SKIP_BUILD!=='1'});try{await seedIngestionAcceptance(lab);await simulate(join(lab.directory,'simulator.json'));console.log('设备接入隔离验收环境：'+lab.origin+'；账号文件：'+join(lab.directory,'accounts.json')+'。Ctrl-C 只停止本环境并清理其专用测试库和桶。');await new Promise(r=>{process.once('SIGTERM',r);process.once('SIGINT',r)});}finally{await lab.close();}
