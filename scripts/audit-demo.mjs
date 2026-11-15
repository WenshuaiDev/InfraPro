import {startAuditLab} from './fixtures/audit-lab.mjs';
import {seedAuditAcceptance} from './fixtures/audit-acceptance.mjs';
import {seedAuditBusiness} from './fixtures/audit-business.mjs';
let lab;try{lab=await startAuditLab({business:true});const state=await seedAuditAcceptance(lab);await seedAuditBusiness(lab,state);console.log(`审计演示：${lab.origin}/access/audit；合成账号见 ${lab.directory}/accounts.json，业务样本见 business-acceptance.json。Ctrl+C 关闭并清理本次数据库与存储。`);for(const signal of ['SIGINT','SIGTERM'])process.once(signal,async()=>{await lab.close();process.exit(0);});await new Promise(()=>{});}catch(e){console.error(e.message);await lab?.close();process.exitCode=1;}
