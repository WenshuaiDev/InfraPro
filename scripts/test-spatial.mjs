import {execFileSync} from 'node:child_process';
import {ensureTestDependencies,testEnv} from './test-environment.mjs';
ensureTestDependencies();
execFileSync('docker',['compose','-f','deploy/monitoring-contract/compose.yaml','up','-d','--wait'],{stdio:'inherit'});
execFileSync('go',['test','-tags','monitoringintegration','./packages/go/platform/resources','-run','^TestSpatial','-count=1','-v'],{stdio:'inherit',env:{...process.env,...testEnv,INFRAPRO_SPATIAL_CONTRACT:'1',INFRAPRO_MONITORING_CONTRACT:'1'}});
execFileSync('.artifacts/spatial-venv/bin/python',['tools/spatial-converter/test_converter.py'],{stdio:'inherit'});
execFileSync('node',['--test','tests/unit/spatial-client.test.mjs'],{stdio:'inherit'});
