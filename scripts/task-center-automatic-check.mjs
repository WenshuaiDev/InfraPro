import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { chromium, expect as baseExpect } from '@playwright/test';
import { taskClient } from './fixtures/tasks-acceptance.mjs';

const directory = process.env.INFRAPRO_TASK_LAB_DIR ?? '.artifacts/task-center/list-detail/lab';
const out = process.env.INFRAPRO_TASK_AUTOMATIC_OUT ?? join(directory,'..');
const json = name => JSON.parse(readFileSync(join(directory, name)));
const state = json('task-acceptance.json');
const expect = baseExpect.configure({ timeout: 30000 });
assert.ok(state.synthetic && state.complete);
const session = json('session.json'), pg = new URL(json('operator.json').database_url);
const schema = pg.searchParams.get('search_path');
assert.equal(session.origin, state.origin);
assert.equal(new URL(state.origin).hostname,'127.0.0.1');
assert.equal(pg.host,'127.0.0.1:55432'); assert.equal(pg.pathname,'/infrapro_test');
assert.match(schema,/^identity_test_[a-z0-9]+$/);
const sql = statement => execFileSync('docker', ['exec','-i','infrapro-account-tests-postgres-1','psql','-X','-q','-v','ON_ERROR_STOP=1','-U','infrapro_test','-d','infrapro_test'], {
  input: 'SET search_path TO "' + schema + '",public;\n' + statement, stdio: ['pipe','pipe','pipe'],
});
const { tenant, channels, range, facility, point } = state.derived, login = state.accounts.B;
const api = await taskClient(state.origin,login);
const browser = await chromium.launch(), page = await browser.newPage({viewport:{width:1440,height:1000}});
const base = '/tenants/' + tenant + '/monitoring/derived', call = (path,body) => api.call(base+path,body);
const meta = revision => ({request_id:randomUUID(),expected_revision:revision,reason:'独立验收 · 自动运行固定目标'});
let installed = false;
try {
  const code = 'VERIFY-' + randomUUID().slice(0,8);
  const { channel } = await call('/channels',{...meta('0'),code,name:'自动运行恢复验收',owner:{kind:'point',facility_id:facility,id:point},
    definition_version_id:channels.A_MINUS_B.version,input_bindings:{A:{channel_id:channels.A.id},B:{channel_id:channels.B.id}},parameters:{}});
  const channelID = channel.resource.id; assert.match(channelID,/^[a-f0-9-]{36}$/);
  const path = '/channels/'+channelID, config = (await call(path+'/configs')).items[0];
  let trial = await call(path+'/trials',{...meta(channel.config_revision),config_id:config.id,config_hash:config.config_hash,output_range:range});
  await expect.poll(async()=>{trial=await call('/runs/'+trial.id);return trial.job.state;}).toBe('succeeded');
  // Only the fresh channel in this disposable E2E schema can encounter the fault.
  // The real worker records the failure, handles both commands and publishes the result.
  sql(`CREATE TABLE task_center_lease_fixture(step integer NOT NULL);
    INSERT INTO task_center_lease_fixture VALUES(0);
    CREATE FUNCTION task_center_lease_fixture_fn() RETURNS trigger LANGUAGE plpgsql AS $fault$
    BEGIN
      IF NEW.kind='derive' AND EXISTS (SELECT 1 FROM derived_runs WHERE job_id=NEW.id AND mode='automatic' AND channel_id='${channelID}'::uuid) THEN
        IF OLD.state='queued' AND NEW.state='running' AND (SELECT step FROM task_center_lease_fixture)=0 THEN
          NEW.lease_until := clock_timestamp()-interval '5 seconds';
          NEW.attempts := 5;
          UPDATE task_center_lease_fixture SET step=1;
        ELSIF OLD.state='failed' AND NEW.state='queued' AND (SELECT step FROM task_center_lease_fixture)=1 THEN
          NEW.available_at := clock_timestamp()+interval '4 minutes';
          UPDATE task_center_lease_fixture SET step=2;
        END IF;
      END IF;
      RETURN NEW;
    END $fault$;
    CREATE TRIGGER task_center_lease_fixture_trg BEFORE UPDATE OF state ON monitoring_jobs FOR EACH ROW EXECUTE FUNCTION task_center_lease_fixture_fn();`);
  installed = true;
  const preview = await call(path+'/control-preview',{expected_revision:channel.control_revision,action:'enable',auto_start_ns:range.from_ns,stop_ns:'1704153600000000000',trial_receipt_id:trial.trial.trial_receipt_id});
  await call(path+'/control',{...meta(channel.control_revision),preview_id:preview.id,preview_hash:preview.hash});
  let failed;
  await expect.poll(async()=>{failed=(await call('/runs?mode=automatic&state=failed&channel_id='+channelID)).items[0];return failed?.job.state;},{timeout:90000}).toBe('failed');
  assert.ok(['job_attempts_exhausted','job_lease_lost'].includes(failed.job.error_code));
  assert.equal(failed.mode,'automatic');
  const limited=await taskClient(state.origin,state.accounts.M);
  try {
    const address=limited.api+base+'/runs/'+failed.id;
    const hidden=await limited.client.get(address,{headers:limited.headers});
    assert.ok([403,404].includes(hidden.status()));
    for(const operation of ['cancel','retry']){
      const denied=await limited.client.post(address+'/'+operation,{headers:{...limited.headers,'X-Command-ID':randomUUID()},data:meta(failed.job.revision)});
      assert.ok([403,404].includes(denied.status()),'Unqualified automatic command '+operation+' returned '+denied.status());
    }
  } finally {await limited.client.dispose();}
  await page.goto(state.origin+'/login');
  await page.getByLabel('用户名',{exact:true}).fill(login.username);await page.getByLabel('密码',{exact:true}).fill(login.password);
  await page.getByRole('button',{name:'登录',exact:true}).click();await page.waitForURL(u=>u.pathname!='/login');
  await page.goto(state.origin+'/tasks?mode=mine&scope_kind=tenant&tenant_id='+tenant);
  await page.getByRole('button',{name:'运行概况',exact:true}).click();
  const panel=page.getByRole('dialog',{name:'运行概况'});
  await panel.getByRole('button',{name:'查看自动运行',exact:true}).click();
  await panel.getByRole('button',{name:new RegExp(failed.id.slice(0,8),'i')}).click();
  await expect(panel.getByRole('heading',{name:'自动计算 · 失败',exact:true})).toBeVisible();
  await expect(panel.getByText(/处理原因：.*(?:租约|job_attempts_exhausted|job_lease_lost)/)).toBeVisible();
  await expect(panel.getByText(/输出范围：/)).toBeVisible();
  await page.screenshot({path:join(out,'自动计算-失败.png')});
  async function control(label,reason,operation) {
    await panel.getByLabel('自动运行操作原因').fill(reason);
    const response=page.waitForResponse(r=>new URL(r.url()).pathname.endsWith('/runs/'+failed.id+'/'+operation) && r.request().method()==='POST');
    await panel.getByRole('button',{name:label,exact:true}).click();
    const http = await response; assert.ok(http.ok(), operation+' status '+http.status()+' '+(http.ok()?'':await http.text()));
    const body=await http.json(); assert.equal(body.id,failed.id);assert.equal(body.job.id,failed.job.id);return body.job.state;
  }
  assert.equal(await control('重试固定目标','租约故障已恢复，重新排队','retry'),'queued');
  await page.setViewportSize({width:390,height:844});
  assert.equal(await control('取消本次自动运行','核对排队状态后取消','cancel'),'cancelled');
  // Observe the cancelled run across multiple scheduler passes before the explicit retry.
  const cancelled = await call('/runs/'+failed.id);
  for (let tick=0; tick<5; tick++) {
    await new Promise(resolve=>setTimeout(resolve,1000));
    const current=await call('/runs/'+failed.id);
    assert.equal(current.job.state,'cancelled');assert.equal(current.job.revision,cancelled.job.revision);
  }
  assert.equal(await control('重试固定目标','复核固定输入后恢复执行','retry'),'queued');
  await expect.poll(async()=>(await call('/runs/'+failed.id)).job.state,{timeout:60000}).toBe('succeeded');
  await expect(panel.getByRole('heading',{name:'自动计算 · 已完成',exact:true})).toBeVisible();
  await page.screenshot({path:join(out,'自动计算-恢复.png')});
  const published=await call(path+'/generations?from_ns='+range.from_ns+'&to_ns='+range.to_ns);
  const fixed=published.items.find(g=>g.run_id===failed.id);assert.ok(fixed);
  const resolved=await call('/runs/'+failed.id);assert.equal(resolved.published_generation_id,fixed.id);
  await panel.getByRole('link',{name:'查看结果',exact:true}).click();
  await page.waitForURL(u=>u.searchParams.get('id')===failed.id&&u.searchParams.get('version')===fixed.id);
  await expect(page.getByRole('table',{name:'固定派生结果明细'})).toBeVisible();
  const inTasks=await api.call('/tasks?q='+failed.id);assert.equal(inTasks.items.length,0);
  for(const legacy of ['/derived-runs?id='+failed.id,'/jobs?job='+failed.job.id]){
    await page.goto(state.origin+'/t/'+tenant+'/monitoring'+legacy);
    await page.waitForURL(u=>u.pathname==='/tasks'&&u.searchParams.get('run')===failed.id);
    await expect(panel.getByRole('heading',{name:'自动计算 · 已完成',exact:true})).toBeVisible();
  }
  const result={passed:true,run:failed.id,job:failed.job.id,channel:channelID,range:failed.output_range,error:failed.job.error_code,states:['failed','queued','cancelled','queued','succeeded'],generation:published.items.find(g=>g.run_id===failed.id).id};
  writeFileSync(join(out,'automatic-check.json'),JSON.stringify(result,null,2));console.log(JSON.stringify(result));
} catch(error) {
  await page.screenshot({path:join(out,'automatic-error.png')}).catch(()=>{});
  console.error(String(error.stack??error).slice(0,2200));process.exitCode=1;
} finally {
  try { if(installed) sql('DROP TRIGGER task_center_lease_fixture_trg ON monitoring_jobs; DROP FUNCTION task_center_lease_fixture_fn(); DROP TABLE task_center_lease_fixture;'); }
  finally {await api.client.dispose();await browser.close();}
}
