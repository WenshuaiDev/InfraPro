import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { digest, read } from './architecture-lab.mjs';

const publicFiles = [
  'result.json',
  'browser/result.json',
  'browser/https-identity.json',
  'nginx.conf',
  'tls/ca.pem',
  'tls/server-chain.pem',
];

function tlsPorts(config) {
  return [...config.matchAll(/^\s*listen 127\.0\.0\.1:(\d+) ssl;/gm)].map(
    (match) => Number(match[1]),
  );
}

export function archiveHTTPSEvidence(directory, work, run) {
  const archive = join('https', run.startedAt.replaceAll(':', '-'));
  const files = {};
  for (const path of publicFiles) {
    const target = join(directory, archive, path);
    mkdirSync(dirname(target), { recursive: true, mode: 0o700 });
    copyFileSync(join(work, path), target);
    files[path] = digest(readFileSync(target));
  }
  const result = read(join(directory, archive, 'result.json'));
  assert.equal(result.passed, true);
  assert.deepEqual(result.browser, read(join(directory, archive, 'browser/result.json')));
  assert.deepEqual(result.identity, read(join(directory, archive, 'browser/https-identity.json')));
  assert(result.browser.passed && result.browser.secureContext && result.browser.checks.length > 0);
  assert(result.identity.passed && result.identity.checks.length > 0);
  assert(['api', 'runtime', 'assets'].every((role) => result.proxyRequests[role] > 0));
  execFileSync('openssl', [
    'verify', '-CAfile', join(directory, archive, 'tls/ca.pem'),
    '-verify_ip', '127.0.0.1', join(directory, archive, 'tls/server-chain.pem'),
  ], { stdio: 'pipe' });
  const originPorts = tlsPorts(readFileSync(join(directory, archive, 'nginx.conf'), 'utf8'));
  assert.equal(originPorts.length, 3);
  assert.equal(new Set(originPorts).size, 3);
  return {
    archive,
    files,
    originPorts,
    nginxConfigSHA256: files['nginx.conf'],
    originalLogSHA256: digest(readFileSync(join(directory, run.log))),
  };
}

export function validHTTPSEvidence(directory, run) {
  const evidence = run.httpsEvidence;
  if (!evidence || !/^https\/\d{4}-[\dTZ.-]+$/.test(evidence.archive) || run.log !== 'test-https.log')
    return false;
  const log = join(directory, run.log);
  const config = join(directory, evidence.archive, 'nginx.conf');
  return existsSync(log) &&
    digest(readFileSync(log)) === evidence.originalLogSHA256 &&
    existsSync(config) &&
    evidence.nginxConfigSHA256 === evidence.files?.['nginx.conf'] &&
    JSON.stringify(tlsPorts(readFileSync(config, 'utf8'))) === JSON.stringify(evidence.originPorts) &&
    publicFiles.every((path) => {
      const file = join(directory, evidence.archive, path);
      return existsSync(file) && digest(readFileSync(file)) === evidence.files?.[path];
    });
}
