import {execFileSync} from 'node:child_process';
import {ensureTestDependencies,testEnv} from './test-environment.mjs';
const run=(cmd,args,env=process.env)=>execFileSync(cmd,args,{stdio:'inherit',env});
ensureTestDependencies();
run('docker',['compose','-f','deploy/monitoring-contract/compose.yaml','up','-d','--wait']);
run('pnpm',['--filter','@infrapro/display-sdk','build']);
run('pnpm',['exec','tsc','--noEmit','--strict','--skipLibCheck','--module','NodeNext','--target','ES2022','tests/types/display-reads.ts']);
run('node',['--test','tests/unit/display-sdk.test.mjs','tests/unit/display-reads.test.mjs','tests/unit/display-package.test.mjs','tests/unit/display-config.test.mjs']);
run('go',['test','-race','-tags','monitoringintegration','-p','2','-timeout','15m','./packages/go/platform/display','./packages/go/platform/displayread','./packages/go/platform/config','./packages/go/platform/monitoring','./packages/go/platform/resources','./services/api/internal/transport/http/display','-run','TestRead|TestDisplay|TestPackageBoundary|TestStrictJSONAndExactInteger|TestSharedRead|TestLastConsumer|TestHTTP|TestRuntime','-count=1','-v'],{...process.env,...testEnv,INFRAPRO_SPATIAL_CONTRACT:'1',INFRAPRO_MONITORING_CONTRACT:'1'});
