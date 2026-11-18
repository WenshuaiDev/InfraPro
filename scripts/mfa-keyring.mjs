// Explicit provisioning only. API startup never creates or replaces keys.
import { randomBytes } from 'node:crypto';
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, isAbsolute } from 'node:path';

export function createMFAKeyring(path) {
  if (!isAbsolute(path)) throw new Error('MFA keyring requires an absolute path outside the repository.');
  mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
  const value = { format_version: 1, active_key_id: 'initial', keys: [{ id: 'initial', key_base64: randomBytes(32).toString('base64') }] };
  writeFileSync(path, JSON.stringify(value) + '\n', { mode: 0o600, flag: 'wx' });
}
