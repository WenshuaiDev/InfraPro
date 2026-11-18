import test from 'node:test';
import assert from 'node:assert/strict';

test('MFA recovery fragments stay isolated and survive anonymous initialization only', async () => {
  const previousWindow = globalThis.window;
  const listeners = new Map();
  const location = { pathname: '/console/mfa-recovery', search: '', hash: '#token=mfa-grant-marker' };
  const replacements = [];
  globalThis.window = {
    location,
    history: { replaceState: (_state, _title, url) => { replacements.push(url); location.hash = ''; } },
    addEventListener: (name, listener) => listeners.set(name, listener),
    get localStorage() { throw Error('credentials must remain in memory'); },
    get sessionStorage() { throw Error('credentials must remain in memory'); },
  };
  try {
    const credential = await import('../../apps/console/src/features/auth/credential.ts');
    assert.deepEqual(replacements, ['/console/mfa-recovery']);
    assert.equal(credential.getCredential('mfa-recovery'), 'mfa-grant-marker');
    credential.observeCredentialIdentity(undefined);
    assert.equal(credential.getCredential('mfa-recovery'), 'mfa-grant-marker');
    for (const purpose of ['invitation', 'reset', 'tenant']) assert.equal(credential.getCredential(purpose), '');
    for (const [route, purpose] of [['invite', 'invitation'], ['reset-password', 'reset'], ['tenant-invite', 'tenant']]) {
      location.pathname = '/console/' + route;
      location.hash = '#token=' + purpose + '-marker';
      listeners.get('hashchange')();
      assert.equal(location.hash, '');
      assert.equal(credential.getCredential(purpose), purpose + '-marker');
      assert.equal(credential.getCredential('mfa-recovery'), 'mfa-grant-marker');
    }
    credential.observeCredentialIdentity('account-a');
    credential.observeCredentialIdentity('account-b');
    assert.equal(credential.getCredential('mfa-recovery'), '');
    assert.equal(credential.getCredential('tenant'), '');
    assert.equal(credential.getCredential('invitation'), 'invitation-marker');
    assert.equal(credential.getCredential('reset'), 'reset-marker');
  } finally {
    if (previousWindow === undefined) delete globalThis.window;
    else globalThis.window = previousWindow;
  }
});
