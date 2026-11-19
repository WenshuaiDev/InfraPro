import assert from 'node:assert/strict';
import { randomBytes } from 'node:crypto';
import { mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { retainedRecordings } from '../../scripts/architecture-cross-console.mjs';

test('two frontends re-open the same retained response, with private data protected', t => {
  const path = mkdtempSync(join(tmpdir(),'shared-console-recording-'));
  t.after(() => rmSync(path,{recursive:true,force:true}));
  const key = randomBytes(32), vault = retainedRecordings(path,key);
  const request = { url:()=>'http://localhost/api/v1/example?cursor=synthetic-cursor',
    method:()=>'GET',postDataBuffer:()=>null,headers:()=>({accept:'application/json'}) };
  const body = Buffer.from('{"synthetic":"unpublished-replay-content"}');
  const item=vault.save(request,{status:200,headers:{'content-type':'application/json','set-cookie':'synthetic-cookie'},body});
  const file=join(path,item.file), bytes=readFileSync(file);
  assert.equal(statSync(file).mode & 0o077,0);
  assert(!bytes.includes(body));
  assert(!JSON.stringify(item).includes('synthetic-cursor'));
  for(let i=0;i<2;i++)assert.deepEqual(retainedRecordings(path,key).restore(item),
    {status:200,headers:{'content-type':'application/json'},body});
  assert.throws(()=>vault.restore({...item,input:'0'.repeat(64)}));
  bytes[bytes.length-1]^=1;writeFileSync(file,bytes);
  assert.throws(()=>vault.restore(item));
});
