import { execFileSync } from 'node:child_process';
import { mkdirSync,openSync,closeSync,readFileSync,writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { ensureTestDependencies,testEnv } from './test-environment.mjs';

ensureTestDependencies();
execFileSync('docker',['compose','-f','deploy/monitoring-contract/compose.yaml','up','-d','--wait'],{stdio:'inherit'});
mkdirSync('.artifacts/audit',{recursive:true});
execFileSync('go',['build','-o','.artifacts/audit/files-checker','./services/api/cmd/files-checker'],{stdio:'inherit'});
const env={...process.env,...testEnv,INFRAPRO_MONITORING_CONTRACT:'1',INFRAPRO_SPATIAL_CONTRACT:'1',INFRAPRO_FILES_CONTRACT:'1',INFRAPRO_FILES_CHECKER_EXECUTABLE:resolve('.artifacts/audit/files-checker'),INFRAPRO_AUDIT_COVERAGE:resolve('.artifacts/audit/producer-observed.jsonl')};
const common=['test','-p','1','-count=1','-v','-timeout=60m'];
const phases={
 spatial:{args:[...common,'-tags','monitoringintegration','./packages/go/platform/resources','-run','^TestSpatial'],extra:{INFRAPRO_AUDIT_SOURCE_ASSERTIONS:'1'}},
 privileges:{args:[...common,'-race','./packages/go/platform/audit','-run','^TestAuditNamedRolesRunAndDenyUnrelatedData$']},
 governance:{args:[...common,...['identity','access','tenancy','projects','engineering','devicecatalog','devices','sources'].map(p=>'./packages/go/platform/'+p),'./services/api/internal/transport/http']},
 resources:{args:[...common,'-tags','monitoringintegration','./packages/go/platform/resources']},
 metadata:{args:[...common,'-tags','monitoringintegration','./packages/go/platform/resources','-run','^Test(DerivedFormalPublicationUsesSealedAttempts|AnalysisAtomicMultiOutputVersionsAndChangedInputs|DisplayLifecycleAndSubjectBoundary|SpatialRealFilesVersionsAndProjectBoundaries|FilesUploadFactsAndImmutableAttachment|AuditRealDisplayHandoffExhaustionAndMaintenanceReplay)$'],extra:{INFRAPRO_AUDIT_SOURCE_ASSERTIONS:'1'}},
 compatibility:{args:[...common,'-race','./packages/go/platform/taskrecord','./packages/go/platform/tasks','./packages/go/platform/notificationevent','./packages/go/platform/notifications','./packages/go/platform/derivedmonitoring'],extra:{INFRAPRO_NOTIFICATIONS_RESTORE:'1'}},
 'resource-repair':{args:[...common,'-race','./packages/go/platform/resources','-run','^TestAuthorizationHistoryInterpretationAndAuditIsolation$']},
 'source-repair':{args:[...common,'-tags','monitoringintegration','./packages/go/platform/resources','-run','^Test(MonitoringRouteCopyPauseTailSwitchAndReverseCopy|SpatialLifecycleProjectDefaultsAndConcurrentDraft)$'],extra:{INFRAPRO_AUDIT_SOURCE_ASSERTIONS:'1'}},
 'derived-metadata':{args:[...common,'-tags','monitoringintegration','./packages/go/platform/resources','-run','^Test(DerivedFormalPublicationUsesSealedAttempts|AnalysisAtomicMultiOutputVersionsAndChangedInputs)$'],extra:{INFRAPRO_AUDIT_SOURCE_ASSERTIONS:'1'}},
 repaired:{args:[...common,'-race','./packages/go/platform/tenancy','./packages/go/platform/projects','./services/api/internal/transport/http','-run','^Test(LifecycleIsolationHistoryAndRecovery|ProjectCatalogPublicationAuditCleanupAndPaging|FilesHTTPRealUploadStrictInputAndGuardedContent)$']},
 'files-restore':{args:[...common,'-race','./packages/go/platform/resources','-run','^TestFilesFrozenBackup'],extra:{INFRAPRO_FILES_RESTORE:'1'}},
};
const selected=process.argv[2]?[process.argv[2]]:['governance','resources','metadata','compatibility','files-restore'];
for(const name of selected){
 const phase=phases[name];if(!phase)throw Error('Unknown audit regression phase: '+name);
 const log=`.artifacts/audit/regression-${name}.log`,fd=openSync(log,'w'),startedAt=new Date().toISOString();
 console.log('Audit regression: '+name);
 let success=false;
 try{execFileSync('go',phase.args,{env:{...env,...phase.extra},stdio:['ignore',fd,fd]});success=true;}
 finally{closeSync(fd);writeFileSync(`.artifacts/audit/regression-${name}.json`,JSON.stringify({phase:name,command:['go',...phase.args],startedAt,completedAt:new Date().toISOString(),passed:success,log},null,2));if(!success)process.stdout.write(readFileSync(log,'utf8').split('\n').slice(-60).join('\n'));}
 console.log('Passed: '+name+'; evidence '+log);
}
