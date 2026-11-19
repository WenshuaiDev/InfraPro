import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { digest, read, write } from './architecture-lab.mjs';
import { sharedConsoleFor } from './architecture-shared-console.mjs';
import { seedWalkthroughFilesDisplay } from './architecture-walkthrough-assets.mjs';
import { seedWalkthroughGovernance } from './architecture-manual.mjs';
import { seedWalkthroughPagination } from './architecture-pagination-sample.mjs';
import { seedWalkthroughDrafts } from './architecture-console-draft-fixture.mjs';
import { seedWalkthroughImportFocus } from './architecture-import-focus-sample.mjs';
import { seedWalkthroughIngestion } from './architecture-ingestion-http.mjs';
import { archiveWalkthroughRecord, observeWalkthrough, verifyWalkthroughHistory } from './architecture-walkthrough-evidence.mjs';

export async function walkthroughInputs(dir, stack) {
  const { lab, side, cfg, manual } = stack;
  assert(manual && ['a', 'b'].includes(side), 'A full walkthrough stack is required');
  const descriptor = join(dir, 'manual', 'walkthrough-' + side + '.json');
  const sharedConsole = sharedConsoleFor(dir, { verifyFiles: true });
  assert.equal(stack.sharedConsoleSHA256, sharedConsole?.console.sha256);
  const local = lab.directory,
    location = relative(dir, local);
  const samples = read(join(local, 'samples.json')), structure = samples.structure;
  const previous = existsSync(descriptor) ? read(descriptor) : null;
  if (previous) {
    if (previous.environment === lab.id) {
      assert.equal(previous.scope.project, structure.project);
      if (previous.execution?.launchID === manual.launchID && previous.releaseSHA256 === stack.releaseSHA256 &&
          (!sharedConsole || (previous.sharedConsoleSHA256 === sharedConsole.console.sha256 &&
            previous.consoleCorrections?.projectPagination && previous.consoleCorrections?.drafts &&
            (!sharedConsole.beforeImportFocus || previous.consoleCorrections.importFocus)))) {
        assert.equal(previous.sharedConsoleSHA256, stack.sharedConsoleSHA256);
        assert.equal(previous.execution.sharedConsoleSHA256, stack.sharedConsoleSHA256);
        return previous;
      }
    }
    archiveWalkthroughRecord(dir, 'walkthrough-' + side, previous);
  }
  const execution = await observeWalkthrough(stack, sharedConsole);
  const accountFile = join(dir, 'private/accounts.json');
  assert.equal(digest(readFileSync(accountFile)), digest(readFileSync(join(local, 'private/accounts.json'))));
  const accounts = read(accountFile);
  const governance = await seedWalkthroughGovernance({
    dir: local,
    credentialDirectory: dir,
    side,
    lab,
    structure,
    accounts,
  });
  const ingestion = await seedWalkthroughIngestion({
    origin: cfg.origin,
    admin: accounts.admin,
    tenant: structure.tenant,
    project: structure.project,
    unit: structure.unit,
    metric: structure.metric,
    relay: manual.relay,
    privateDir: manual.folder,
  });
  const assets = await seedWalkthroughFilesDisplay({
    directory: local,
    side,
    lab: { ...cfg, config: manual.operatorConfig },
    structure,
    account: accounts.admin,
    workers: 'external',
    verifyTerminal: true,
  });
  assert(ingestion.receipt_id && ingestion.result_id && ingestion.raw_bytes > 0);
  assert(assets.files.receiving.nextPart === 2 && assets.files.readable.revisionID);
  assert(
    assets.spatial.layout.version &&
      assets.spatial.model.version &&
      assets.display.release &&
      assets.display.terminal,
  );
  const historyReadback = await verifyWalkthroughHistory(cfg, accounts, samples, governance);
  const projectPagination = sharedConsole && await seedWalkthroughPagination({
    lab: cfg, account: accounts.admin, scope: structure.tenant,
  });
  const drafts = sharedConsole && await seedWalkthroughDrafts({
    directory: dir, environment: lab.id, side, cfg, admin: accounts.admin,
  });
  const importFocus = sharedConsole?.beforeImportFocus &&
    await seedWalkthroughImportFocus({ lab: cfg, account: accounts.admin, tenant: structure.tenant });
  const data = {
    receipt: ingestion.receipt_id,
    unit: ingestion.unit_id,
    result: ingestion.result_id,
    rawSHA256: ingestion.raw_sha256,
    facility: governance.sample.facility,
    history: [governance.version1, governance.version2],
    attachment: assets.files.readable.openPath,
    resumableUpload: assets.files.receiving.openPath,
    layout: assets.spatial.layout.openPath,
    model: assets.spatial.model.openPath,
    display: assets.display.openPath,
    terminal: assets.display.terminal,
  };
  const comparison = previous && previous.environment === lab.id ? Object.fromEntries(
    Object.entries(data).map(([key, value]) => [key, JSON.stringify(value) === JSON.stringify(previous.data[key]) ? 'rechecked' : 'replaced']),
  ) : null;
  const result = {
    environment: lab.id,
    side,
    scope: governance.scope,
    preparedAt: new Date().toISOString(),
    console: cfg.origin,
    runtime: cfg.runtime,
    diagnostics: 'http://127.0.0.1:' + cfg.ports.diagnostics,
    roles: Object.keys(execution.processes),
    governance: join(location, 'manual', side + '.json'),
    restrictedProjectRows: governance.limitedProjectViewer.count,
    data,
    laboratory: location,
    releaseSHA256: stack.releaseSHA256,
    ...(sharedConsole && { sharedConsoleSHA256: sharedConsole.console.sha256,
      consoleCorrections: { projectPagination, drafts, ...(importFocus && { importFocus }) } }),
    execution,
    sampleReadback: {
      checkedAt: new Date().toISOString(),
      comparedToRelease: previous?.releaseSHA256,
      comparison,
      permittedRows: governance.limitedProjectViewer.count,
      deniedCrossProject: governance.limitedProjectViewer.crossProjectStatus,
      rawBytes: ingestion.raw_bytes,
      attachmentSHA256: assets.files.readable.sourceSHA256,
      nextUploadPart: assets.files.receiving.nextPart,
      layoutSHA256: assets.spatial.layout.sourceSHA256,
      modelSHA256: assets.spatial.model.sourceSHA256,
      terminalRelease: assets.display.release,
      history: historyReadback,
      notificationTask: governance.noticeTask,
      invitation: governance.invitationReadback,
    },
    credentials: {
      people: 'private/accounts.json',
      project: governance.credentialFile,
      diagnostics: join(location, 'private', side, 'diagnostics/accounts.json'),
      ...(drafts && { drafts: drafts.credentialFile }),
    },
    privateMaterials: join(location, 'private', side, 'walkthrough-assets/manifest.json'),
    manualAcceptance: 'pending',
  };
  write(descriptor, result);
  if (side === 'b') {
    const saved = join(dir, 'manual', 'walkthrough-candidate.json');
    const selected = { directory: dirname(stack.bin), sourceSHA256: stack.releaseSHA256 };
    if (existsSync(saved)) {
      const earlier = read(saved);
      if (earlier.directory !== selected.directory || earlier.sourceSHA256 !== selected.sourceSHA256) {
        archiveWalkthroughRecord(dir, 'walkthrough-candidate', earlier, previous?.environment ?? lab.id, earlier.sourceSHA256);
      }
    }
    write(saved, selected);
  }
  return result;
}
