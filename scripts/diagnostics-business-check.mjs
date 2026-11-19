import { execFileSync } from 'node:child_process';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { openSync, closeSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { loadLab, readJSON, root, cleanEnv, command, ownedContainers, checkContainerOwnership, privateJSON } from './fixtures/diagnostics-lab.mjs';
const lab=loadLab();if(!lab.business||lab.faults.length)throw Error('Recover the complete owned business lab first');
const cfg=readJSON(lab.business_config),db=new URL(cfg.database_url);db.pathname='/diagnostics_lab_test';
const pg=ownedContainers(lab).find(id=>JSON.parse(command('docker',['inspect','--format','{{json .Config.Labels}}',id]))['io.infrapro.replica']==='postgres-1');checkContainerOwnership(lab,pg);
if(!command('docker',['exec',pg,'psql','-U','diagnostics_lab','-d','diagnostics_lab','-At','-c',"SELECT 1 FROM pg_database WHERE datname='diagnostics_lab_test'"]).trim())command('docker',['exec',pg,'createdb','-U','diagnostics_lab','diagnostics_lab_test']);
const releaseBin = process.env.INFRAPRO_DIAGNOSTICS_SEALED_BIN;
const residentDirectory = join(lab.directory, 'bin');
const output = releaseBin ? mkdtempSync(join(lab.directory, 'owner-compile-')) : residentDirectory;
const roles = readJSON(join(root, 'contracts/diagnostics/runtime-catalog.json')).entries
  .filter(role => role.entry_point?.startsWith('services/api/cmd/'));
const checkFile = file => createHash('sha256').update(readFileSync(file)).digest('hex');
try {
  for (const role of roles) {
    execFileSync('go', ['build', '-trimpath', '-o', join(output, role.id), './services/api/cmd/' + role.id], { cwd: root, stdio: 'inherit' });
  }
  if (releaseBin) {
    const registered = roles.filter(role => lab.processes[role.id]?.executable === join(residentDirectory, role.id));
    assert(registered.length > 0);
    privateJSON(join(lab.directory, 'evidence/owning-process-builds.json'), {
      run_id: lab.run_id, recorded_at: new Date().toISOString(), built_roles: roles.map(role => role.id),
      owners: registered.map(({ id }) => {
        const selected = checkFile(join(releaseBin, id)), served = checkFile(join(residentDirectory, id));
        assert.equal(served, selected, 'Registered role differs from the selected release: ' + id);
        return { role: id, selected_sha256: selected, served_sha256: served, independent_compile_sha256: checkFile(join(output, id)) };
      }),
    });
  }
} finally {
  if (releaseBin) rmSync(output, { recursive: true, force: true });
}
const log=join(lab.directory,'evidence/owning-processes.log'),fd=openSync(log,'w',0o600);
try{execFileSync('go',['test','-count=1','-v','-timeout','8m','-tags','diagnosticsintegration monitoringintegration','./packages/go/platform/resources','-run','^TestDiagnosticsActualOwningProcesses$'],{cwd:root,env:cleanEnv({INFRAPRO_TEST_DATABASE_URL:db.toString(),INFRAPRO_TEST_REDIS_URL:cfg.redis_url,INFRAPRO_DIAGNOSTICS_LAB_DIRECTORY:lab.directory}),stdio:['ignore',fd,fd]});console.log(`Actual owning-process integration passed; evidence: ${log}`)}catch{throw Error(`Actual owning-process integration failed; private evidence: ${log}`)}finally{closeSync(fd)}
