import { execFile } from 'node:child_process';
import { readFile } from 'node:fs/promises';
import { freemem, loadavg, platform, totalmem } from 'node:os';
import { promisify } from 'node:util';

const command=promisify(execFile);
export async function hostPressure(rolePIDs){
  const snapshot={memoryBytes:totalmem(),freeBytes:freemem(),load:loadavg()};
  if(platform()==='darwin'){
    const {stdout}=await command('vm_stat');
    snapshot.pageBytes=Number(stdout.match(/page size of (\d+) bytes/)?.[1]);
    snapshot.pages=Object.fromEntries([...stdout.matchAll(/^([^:]+):\s+(\d+)\./gm)]
      .filter(([,key])=>['Pages free','Pages wired down','Pages occupied by compressor','Compressions','Decompressions','Pageouts','Swapins','Swapouts'].includes(key.trim()))
      .map(([,key,value])=>[key.trim(),Number(value)]));
  }else if(platform()==='linux'){
    const text=await readFile('/proc/meminfo','utf8');
    snapshot.vmKiB=Object.fromEntries([...text.matchAll(/^(MemAvailable|SwapFree|SwapTotal):\s+(\d+) kB/gm)].map(([,key,value])=>[key,Number(value)]));
  }
  const {stdout}=await command('ps',['-A','-o','pid=,%cpu=,comm=']);
  const cpu={roles:0,docker:0,go:0,node:0,browser:0,other:0};
  for(const line of stdout.split('\n')){
    const cells=line.trim().match(/^(\d+)\s+([\d.]+)\s+(.+)$/);if(!cells)continue;
    const [,pid,percent,exe]=cells;
    let bucket='other';
    if(rolePIDs.has(Number(pid)))bucket='roles';
    else if(/docker|virtualization|qemu|virtiofs/i.test(exe))bucket='docker';
    else if(/chrome|chromium|firefox|webkit/i.test(exe))bucket='browser';
    else if(/\/(go|compile|link|[^/]+\.test)$/i.test(exe))bucket='go';
    else if(/\/(node|pnpm)(?:$| )/i.test(exe))bucket='node';
    cpu[bucket]+=Number(percent);
  }
  snapshot.cpuPercent=Object.fromEntries(Object.entries(cpu).map(([key,value])=>[key,Math.round(value*10)/10]));
  return snapshot;
}
export function databasePressure(sql){
  return JSON.parse(sql(`WITH active AS MATERIALIZED (
    SELECT state,wait_event_type,wait_event,pid FROM pg_stat_activity
    WHERE datname=current_database() AND pid<>pg_backend_pid()
  ) SELECT jsonb_build_object(
    'connections',(SELECT count(*) FROM active),
    'running',(SELECT count(*) FROM active WHERE state='active'),
    'blocked',(SELECT count(*) FROM active WHERE wait_event_type='Lock' AND cardinality(pg_blocking_pids(pid))>0),
    'waits',(SELECT coalesce(jsonb_object_agg(event,n),'{}') FROM
      (SELECT coalesce(wait_event_type||'/'||wait_event,'on-cpu') AS event,count(*) AS n FROM active WHERE state='active' GROUP BY 1) w),
    'disk',(SELECT jsonb_build_object('readMS',blk_read_time,'writeMS',blk_write_time,'readBlocks',blks_read,'hitBlocks',blks_hit,'tempBytes',temp_bytes,'deadlocks',deadlocks) FROM pg_stat_database WHERE datname=current_database())
  )`));
}

// Persist only aggregate work counts and timings. The database statistics view
// can contain test literals and is never written to the evidence directory.
export function databaseStatements(sql){
  const rows=JSON.parse(sql(`SELECT coalesce(jsonb_agg(to_jsonb(t)),'[]')
    FROM (SELECT md5(query) AS fingerprint,
      CASE WHEN query ~* 'monitoring_summary_replacements' THEN 'summary replacement'
           WHEN query ~* 'display_read_jobs' THEN 'display job'
           WHEN query ~* 'display_read_quer' THEN 'display query or rows'
           WHEN query ~* 'monitoring_selection|monitoring_query_snap' THEN 'monitoring selection or snapshot'
           WHEN query ~* 'monitoring_(channel|source|observ)' THEN 'monitoring source'
           WHEN query ~* 'account_session|authorization_state|permission_catalog' THEN 'authority'
           WHEN query ~* '^(BEGIN|COMMIT|ROLLBACK|SET )' THEN 'transaction'
           ELSE 'other' END AS family,
       calls,total_exec_time AS ms,rows,shared_blks_read AS blocks_read,
       temp_blks_written AS temp_written
    FROM public.pg_stat_statements WHERE dbid=(SELECT oid FROM pg_database WHERE datname=current_database())
      AND query NOT ILIKE '%pg_stat_statements%') t`));
  return Object.fromEntries(rows.map(row=>[row.fingerprint,row]));
}
export function statementDelta(before,after){
  const rows=Object.entries(after).map(([fingerprint,current])=>{
    const initial=before[fingerprint]||{};
    return {fingerprint,family:current.family,calls:current.calls-(initial.calls||0),rows:current.rows-(initial.rows||0),
      ms:current.ms-(initial.ms||0),blocksRead:current.blocks_read-(initial.blocks_read||0),tempWritten:current.temp_written-(initial.temp_written||0)};
  }).filter(s=>s.calls>0).sort((a,b)=>b.ms-a.ms);
  const families={};
  for(const row of rows){
    const aggregate=families[row.family]??={calls:0,ms:0,rows:0,blocksRead:0,tempWritten:0};
    for(const key of Object.keys(aggregate))aggregate[key]+=row[key];
  }
  return {families,top:rows.slice(0,20)};
}
