import assert from 'node:assert/strict';
import {
  closeSync,
  existsSync,
  mkdirSync,
  openSync,
  readFileSync,
  unlinkSync,
  writeFileSync,
} from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { checks, readChecks, regression } from './architecture-regression.mjs';
import { runDirectory, write } from './architecture-lab.mjs';
import { performanceStages, registerFunctionalScope, satisfied, skipReason } from './architecture-acceptance-scope.mjs';

const order = [
  'contracts:check',
  'permissions:check',
  'test:monitoring:client',
  'test:files:client',
  'test:notifications:client',
  'test:files:sandbox',
  'test:display:compatibility',
  'check',
  'build',
  'test:mfa',
  'test:parsing',
  'test:parsing:regression',
  'test:ingestion',
  'test:ingestion:load',
  'test:monitoring:contract',
  'test:derived',
  'test:derived:contract',
  'test:derived:database',
  'test:derived:load',
  'test:derived:pipeline',
  'test:derived:soak',
  'test:spatial',
  'test:display',
  'test:display:plans',
  'test:display:load',
  'test:uploads',
  'test:files',
  'test:files:http',
  'test:tasks',
  'test:notifications',
  'test:notifications:domains',
  'test:audit',
  'test:audit:regression',
  'test:http',
  'test:https',
  'test:monitoring',
  'test:monitoring:restore',
  'test:files:restore',
  'test:display:restore',
  'test:notifications:restore',
  'test:audit:restore',
  'test:identity',
  'test:e2e:preview',
  'test:diagnostics',
];
assert.deepEqual([...order].sort(), [...checks].sort());

function resource(command) {
  if (command === 'test:http') return 'nginx-http-18088';
  if (command === 'test:diagnostics') return 'diagnostic-fault-lab';
  if (command === 'test:display:compatibility') return 'display-sdk-listeners';
  return null;
}
export function regressionQueue(dir) {
  const scope = registerFunctionalScope(dir);
  const result = { updated: new Date().toISOString(), scope: scope.scope, commandsPerRelease: order.length, sides: {} };
  for (const side of ['baseline', 'candidate']) {
    const previous = new Map(readChecks(dir, side).map((row) => [row.command[1], row.status]));
    result.sides[side] = order.map((command) => ({
      command,
      status: previous.get(command),
      ...(command === 'test:display:load' && { arguments: ['--boundary'] }),
      ...(performanceStages[command] && { skippedStages: performanceStages[command].map(name => ({ name, status: 'skipped', skipReason })) }),
      ...(previous.get(command) === 'skipped' && { skipReason }),
      shared: resource(command),
    }));
  }
  write(join(dir, 'regression/queue.json'), result);
  return result;
}
function lock(file) {
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      const descriptor = openSync(file, 'wx', 0o600);
      try {
        writeFileSync(descriptor, String(process.pid));
      } finally {
        closeSync(descriptor);
      }
      return true;
    } catch (error) {
      if (error.code !== 'EEXIST') throw error;
    }
    const owner = Number(readFileSync(file, 'utf8'));
    if (!owner) return false;
    try {
      process.kill(owner, 0);
      return false;
    } catch (error) {
      if (error.code !== 'ESRCH') throw error;
    }
    try {
      unlinkSync(file);
    } catch (error) {
      if (error.code !== 'ENOENT') throw error;
    }
  }
  return false;
}
async function remaining(dir, side) {
  assert(['baseline', 'candidate'].includes(side));
  const folder = join(dir, 'regression/locks');
  mkdirSync(folder, { recursive: true, mode: 0o700 });
  const own = join(folder, side);
  assert(lock(own), 'A runner for this release is already active');
  const pause = join(dir, 'regression', 'pause');
  process.env.ARC_BROWSER_CHECKPOINT_FILE ??= pause;
  const errors = [];
  try {
    for (const row of regressionQueue(dir).sides[side]) {
      if (existsSync(pause)) break;
      if (satisfied(row)) continue;
      const shared = row.shared && join(folder, row.shared);
      if (shared && !lock(shared)) continue;
      try {
        await regression(dir, side, [row.command], { stopStorage: true });
      } catch (error) {
        errors.push({ command: row.command, message: error.message.slice(0, 300) });
      } finally {
        if (shared) unlinkSync(shared);
      }
      regressionQueue(dir);
    }
  } finally {
    unlinkSync(own);
  }
  const pending = regressionQueue(dir).sides[side].filter((row) => !satisfied(row));
  write(join(dir, 'regression', side, 'queue-state.json'), {
    at: new Date().toISOString(),
    paused: existsSync(pause),
    pending,
    errors,
  });
  return !pending.length && !errors.length;
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const dir = runDirectory(process.argv[2]),
    side = process.argv[3];
  if (!side) regressionQueue(dir);
  else
    remaining(dir, side)
      .then((passed) => {
        process.exitCode = passed ? 0 : 1;
      })
      .catch((error) => {
        console.error(error);
        process.exitCode = 1;
      });
}
