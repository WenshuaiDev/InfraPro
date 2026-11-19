import { execFileSync, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, writeFileSync, readFileSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { checkoutApplicationDigest } from './architecture-runtime.mjs';
import { testEnv, ensureTestDependencies } from './test-environment.mjs';

const out = resolve(process.env.INFRAPRO_ARCHITECTURE_RUN_DIR ?? process.env.ARTIFACT_DIR ?? '.artifacts/architecture-refactor/events-local', 'events');
mkdirSync(out, { recursive: true, mode: 0o700 });
const run = (...args) => execFileSync(...args).toString().trim();
const goos = run('go', ['env', 'GOOS']);
const goarch = run('go', ['env', 'GOARCH']);
const binary = resolve('build', 'events', goos + '-' + goarch, 'nats-server');
if (!existsSync(binary)) execFileSync('node', ['scripts/architecture-broker-build.mjs', goos, goarch], {stdio:'inherit'});
const version = run(binary,['--version']);
if (version !== 'nats-server: v2.14.6') throw Error('Unexpected offline broker version: '+version);
const fileDigest = (path) => createHash('sha256').update(readFileSync(path)).digest('hex');
const source = () => ({
  applicationSHA256: checkoutApplicationDigest(process.cwd()),
  brokerSHA256: fileDigest(binary),
  eventGrantsSHA256: fileDigest('deploy/events/postgres-grants.sql'),
  roleGrantsSHA256: fileDigest('deploy/architecture/postgres-roles.sql'),
});
const before = source();
const release = process.env.INFRAPRO_ARCHITECTURE_HTTP_RELEASE;
const expectedSHA = release && JSON.parse(readFileSync(join(release, 'candidate.json'))).inputs.sourceSHA256;
if (expectedSHA && before.applicationSHA256 !== expectedSHA)
  throw Error('Event checks must run from the selected candidate source.');
ensureTestDependencies();
const env = {...process.env,...testEnv,INFRAPRO_ARCHITECTURE_EVENTS_TEST:'1',INFRAPRO_TEST_REPO_ROOT:process.cwd(),INFRAPRO_EVENT_TEST_SERVER:binary,INFRAPRO_EVENT_TEST_EVIDENCE:out};
const start=new Date();
const args=['test','-count=1','-timeout=5m','-json','./packages/go/platform/eventbus'];
const result=spawnSync('go',args,{env,encoding:'utf8',maxBuffer:32<<20});
const file=join(out,'go-test.jsonl');
writeFileSync(file,result.stdout ?? '',{mode:0o600});
const records=readFileSync(file,'utf8').trim().split('\n').filter(Boolean).map(line=>JSON.parse(line));
const cases=records.filter(x=>x.Test&&['pass','fail','skip'].includes(x.Action)).map(x=>({name:x.Test,state:x.Action,seconds:x.Elapsed}));
const expected=['TestVersionedWireContract','TestFinalDeliverySupervision','TestRestrictedDatabaseRoles','TestAllStreamDiskWatermarks','TestContinuousPositionsAndBackpressure','TestMTLSRuntimeFallback','TestIdentifyHandoffResumesAfterLostExecutor','TestParseHandoffResumesAfterLostExecutor','TestTransactionalOutboxFaults','TestEpochAndDispatchFences','TestJetStreamQuorumAndCapacity','TestTenantQueueSamplesCurrentEpoch','TestPostgresWarmDrain'];
const after = source();
const sourcesUnchanged = Object.keys(before).every((key) => before[key] === after[key]);
const passed=sourcesUnchanged && result.status===0 && expected.every(name=>cases.some(item=>item.name===name&&item.state==='pass')) && cases.every(item=>item.state==='pass');
writeFileSync(join(out,'checks.json'),JSON.stringify({command:['go',...args],started:start.toISOString(),duration_ms:Date.now()-start,exit_code:result.status,broker:version,provenance:{expectedSHA,before,after,sourcesUnchanged},cases,passed},null,2)+'\n',{mode:0o600});
for(const entry of cases)console.log(entry.name,entry.state,entry.seconds+'s');
if(!passed){console.error(records.filter(x=>x.OutputType==='error'||x.Action==='build-output'||x.Action==='fail').map(x=>x.Output??x.BuildOutput??x.Test).join(''));console.error(result.stderr??'');process.exit(1)}
console.log(file);
