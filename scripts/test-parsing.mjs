import {execFileSync} from 'node:child_process';
import {mkdirSync,writeFileSync,mkdtempSync} from 'node:fs';
import {resolve,join} from 'node:path';
import {ensureTestDependencies,testEnv} from './test-environment.mjs';
import {startParser} from './fixtures/parsing-lab.mjs';
const out=resolve('.artifacts/parsing');mkdirSync(out,{recursive:true});ensureTestDependencies();execFileSync('docker',['compose','-f','deploy/monitoring-contract/compose.yaml','up','-d','--wait'],{stdio:'inherit'});
const parser=await startParser(mkdtempSync(join(out,'qualified-'))),checks=[];
const env={...process.env,...testEnv,INFRAPRO_PARSING_URL:parser.url,INFRAPRO_PARSING_TOKEN_FILE:parser.token,INFRAPRO_PARSER_TEST_RUNNER:parser.runner,INFRAPRO_PARSER_TEST_PACKAGE:parser.pkg};
const run=(name,args)=>{const start=Date.now();try{const log=execFileSync('go',args,{env,encoding:'utf8',maxBuffer:16*1024*1024});writeFileSync(join(out,name+'.log'),log);checks.push({name,passed:true,seconds:(Date.now()-start)/1000});console.log(name+' passed');}catch(e){writeFileSync(join(out,name+'.log'),String(e.stdout??'')+String(e.stderr??''));checks.push({name,passed:false});throw Error(name+' failed; inspect .artifacts/parsing/'+name+'.log',{cause:e});}};
try{run('service-tests',['test','./services/parser/...','./packages/go/foundation/parsingcontract','-v','-count=1']);run('fuzz',['test','./services/parser/internal/methods','-run','^$','-fuzz','FuzzDDP','-fuzztime','5s']);run('platform-tests',['test','./packages/go/platform/parsing','-v','-count=1','-timeout','5m']);run('http-tests',['test','./services/api/internal/transport/http','-run','TestParsing|TestRoutePolicyCoverage','-v','-count=1']);}finally{await parser.close();writeFileSync(join(out,'tests.json'),JSON.stringify({checks,allPassed:checks.length===4&&checks.every(c=>c.passed),synthetic:true},null,2));}
