import {execFileSync} from 'node:child_process';
import {ensureTestDependencies,testEnv} from './test-environment.mjs';
ensureTestDependencies();
execFileSync('go',['test','-race','-count=1','-v','-timeout=10m','./packages/go/platform/notifications','-run','^TestUpgradeFrom35AndIndependentNotificationRestore$'],{stdio:'inherit',env:{...process.env,...testEnv,INFRAPRO_NOTIFICATIONS_RESTORE:'1'}});
