import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {dirname,resolve} from 'node:path';

const root=resolve('.artifacts/audit'),checks=[];
const read=file=>readFileSync(resolve(root,file),'utf8');
const json=file=>JSON.parse(read(file));
const requireCheck=(name,passed,evidence)=>{checks.push({name,passed,evidence});if(!passed)throw Error('Delivery check failed: '+name);};
try{
 requireCheck('Core audit, HTTP and client tests',/ℹ fail 0/.test(read('core-final.log'))&&!/^--- (FAIL|SKIP): TestAudit(?!WorkerProcess)/m.test(read('domain.log')),'core-final.log / domain.log');
 requireCheck('Latest client and registered source routes',/ℹ pass 6/.test(read('client-final.log'))&&/ℹ fail 0/.test(read('client-final.log')),'client-final.log');
 requireCheck('Lint, typecheck, Go and generated contract check',read('check-final.log').includes('Permission catalog and generated files are synchronized.'),'check-final.log');
 requireCheck('Production build',read('build-final.log').includes('built in')&&!/ERR_PNPM|error TS\d|FAIL/.test(read('build-final.log')),'build-final.log');
 requireCheck('OpenAPI and source contracts generated',read('contracts-final.log').includes('Generated API types.'),'contracts-final.log');
 for(const phase of ['repaired','resource-repair','source-repair','derived-metadata','compatibility','files-restore','spatial','privileges'])requireCheck('Regression '+phase,json('regression-'+phase+'.json').passed===true,'regression-'+phase+'.json');
 for(const file of ['browser/result.json','history-browser/result.json','contract-guard.json'])requireCheck(file,json(file).passed===true,file);
 const history=json('history-browser/result.json');requireCheck('Every source-history journey completed in the full sweep',history.focused===false&&history.checks.length===36&&history.checks.every(check=>check.passed),'history-browser/result.json');
 for(const name of ['current','legacy']){
  const log=read('monitoring-restore-'+name+'.log'),directory=log.match(/Independent restore evidence: (.+)/)?.[1];
  requireCheck('Independent monitoring restore '+name,!!directory&&JSON.parse(readFileSync(resolve(directory,'result.json'),'utf8')).status==='passed','monitoring-restore-'+name+'.log');
  if(name==='legacy')for(const test of ['TestDerivedUpgradeFromLegacyBackup','TestLegacyRejectsNewMigration'])requireCheck(test,log.includes('--- PASS: '+test+' ('),'monitoring-restore-legacy.log');
 }
 const session=json('lab/session.json');requireCheck('Synthetic business demo complete',session.state==='running'&&session.business&&json('lab/business-acceptance.json').complete,'lab/session.json / lab/business-acceptance.json');
 for(const path of ['/healthz','/readyz']){const r=await fetch(session.apiOrigin+path,{signal:AbortSignal.timeout(5000)});requireCheck('Demo '+path,r.ok,'local HTTP '+path);}
 requireCheck('Console available',(await fetch(session.origin,{signal:AbortSignal.timeout(5000)})).ok,'local console');
 const documents=['docs/README.md','docs/requirements/audit-and-operation-tracing-development-plan.md','docs/verification/audit-and-operation-tracing.md','docs/operations/audit-and-operation-tracing.md','docs/operations/audit-and-operation-tracing-acceptance.md','docs/development/audit-integration.md','docs/development/audit-event-catalog.md'];
 const missing=[];
 for(const doc of documents)for(const [,link]of readFileSync(doc,'utf8').matchAll(/\]\(([^)]+)\)/g)){
  if(link.includes('://')||link.startsWith('#'))continue;
  const target=resolve(dirname(doc),link.split('#')[0]);if(target===resolve(root,'delivery-checks.json'))continue;
  if(!existsSync(target))missing.push(doc+': '+link);
 }
 requireCheck('All delivery document links resolve',missing.length===0,missing.length?missing:documents);
 writeFileSync(resolve(root,'delivery-checks.json'),JSON.stringify({passed:true,checks,completedAt:new Date().toISOString(),manualAcceptance:'pending'},null,2));console.log('Delivery evidence passed: '+checks.length+' checks');
}catch(error){writeFileSync(resolve(root,'delivery-checks.json'),JSON.stringify({passed:false,checks,error:String(error),completedAt:new Date().toISOString()},null,2));throw error;}
