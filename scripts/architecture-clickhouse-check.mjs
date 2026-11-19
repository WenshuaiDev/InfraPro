import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash,randomBytes } from 'node:crypto';
import { readFileSync,writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { clickhouseProfile } from './architecture-clickhouse.mjs';
import { inventory } from './architecture-runtime.mjs';

export function checkAnalytics(layout,container,database){
  assert.match(container,/^[a-f0-9]{64}$/);assert.equal(database,'arc_joint');
  const run=(sql,user,password)=>{
    const args=['exec','-i',...(user?['-e','CLICKHOUSE_PASSWORD']:[]),container,'clickhouse-client','--multiquery',...(user?['--user',user]:[])];
    return execFileSync('docker',args,{input:sql,encoding:'utf8',stdio:'pipe',env:{...process.env,CLICKHOUSE_PASSWORD:password??''}}).trim();
  };
  const report=[];
  for(const role of Object.keys(inventory.roles)){
    const policy=clickhouseProfile(role,database);if(!policy)continue;
    const mount=join(layout,'roles',role),issued=readFileSync(join(mount,'clickhouse.sql'),'utf8');assert.equal(issued,policy.sql);
    const user='arc_analytics_'+randomBytes(8).toString('hex'),password=randomBytes(32).toString('hex');
    const hash=createHash('sha256').update(password).digest('hex');
    run(issued+'CREATE USER '+user+" IDENTIFIED WITH sha256_hash BY '"+hash+"';\nGRANT "+policy.principal+' TO '+user+';');
    const file=join(mount,'environment.json');
    let env={};try{env=JSON.parse(readFileSync(file));}catch(e){if(e.code!=='ENOENT')throw e;}
    writeFileSync(file,JSON.stringify({...env,INFRAPRO_MONITORING_CLICKHOUSE_USER:user,INFRAPRO_MONITORING_CLICKHOUSE_PASSWORD:password})+'\n',{mode:0o600});
    const activity=(sql)=>run(sql,user,password);
    for(const table of ['observations','derived_results_local'])assert.match(activity('SELECT count() FROM '+database+'.'+table),/^\d+$/);
    const rights={};
    for(const [table,writers] of [['observations',['monitoring-worker','monitoring-publish-worker']],['derived_results_local',['monitoring-worker','monitoring-incremental-worker','monitoring-history-worker']]]){
      const sql='INSERT INTO '+database+'.'+table+' SELECT * FROM '+database+'.'+table+' LIMIT 0';
      const allowed=writers.includes(role);if(allowed)activity(sql);else assert.throws(()=>activity(sql),e=>e.stderr?.toString().includes('ACCESS_DENIED'));
      rights[table]=allowed;
    }
    assert.equal(activity('CHECK GRANT DROP TABLE ON '+database+'.observations'),'0');
    report.push({role,selectRows:true,insert:rights,canDropRawTable:false});
  }
  return {status:'passed',roles:report};
}
