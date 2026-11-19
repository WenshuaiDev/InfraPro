// Render the limits already enforced by the released processes. This module
// does not set a second budget or open a connection to a business service.
import { existsSync, readFileSync } from 'node:fs';
import { basename, join } from 'node:path';
import { httpOwners, inventory, writeRuntimeJSON } from './architecture-runtime.mjs';
import { processCapacity, processTotals } from './architecture-process-resources.mjs';

const originalStorageRoles = ['api','monitoring-worker','display-runtime','ingest-worker','spatial-worker'];
const privateHTTP = new Set([...httpOwners,'compat-api','display-runtime','display-assets','monitoring-incremental-worker']);
const defaultTopology = {source:'production-single-endpoint',groups:[{group:'local',replicas:1}]};
function json(path) {
  const raw=readFileSync(path,'utf8');
  try { return JSON.parse(raw); }
  catch { throw Error('Invalid JSON in '+basename(path)); }
}

function whole(value, min, max, name) {
  const n = Number(value);
  if (!Number.isSafeInteger(n) || n < min || n > max) throw Error('Invalid '+name);
  return n;
}

function physicalReplicas(value) {
  if (!Array.isArray(value)) throw Error('Expected an array of physical ClickHouse replicas');
  const addresses=value.map(value=>{
    const parts=typeof value==='string' && /^(\[[^\]]+\]|[^:\s/@?#]+):([0-9]+)$/.exec(value.trim());
    if (!parts || Number(parts[2])<1 || Number(parts[2])>65535) throw Error('Invalid physical ClickHouse endpoint');
    return parts[1].toLowerCase()+':'+Number(parts[2]);
  });
  if (new Set(addresses).size!==addresses.length) throw Error('Duplicate physical replica within a ClickHouse group');
  return addresses.length;
}

// The original monitoring config stores the central comma list and the shard
// JSON as two strings. The same address in two different shard databases
// creates two clickhouse-go pools, including when a node also serves "local".
function topology(config, explicit, source) {
  const central=config.monitoring_clickhouse_replicas, encoded=config.monitoring_physical_shards;
  if (!explicit && central===undefined && encoded===undefined) return defaultTopology;
  if (central!==undefined && typeof central!=='string') throw Error('monitoring_clickhouse_replicas must be a comma-separated string');
  const replicas=physicalReplicas(central ? central.split(',') : []);
  let shards=encoded??[];
  if (typeof encoded==='string') {
    try { shards=JSON.parse(encoded||'[]'); }
    catch { throw Error('Invalid monitoring physical shard JSON'); }
  }
  if (!Array.isArray(shards) || shards.length>32 || (shards.length && !replicas)) throw Error('Invalid monitoring physical shards');
  const groups=replicas ? [{group:'local',replicas}] : [];
  const names=new Set(['local']), databases=new Set([config.monitoring_clickhouse_database]);
  for (const shard of shards) {
    if (!shard || !/^[a-z][a-z0-9_]{0,62}$/.test(shard.name??'') || !/^[a-z][a-z0-9_]{0,62}$/.test(shard.database??'') ||
        names.has(shard.name) || databases.has(shard.database) || !shard.replicas?.length) throw Error('Invalid or duplicate physical shard');
    names.add(shard.name); databases.add(shard.database);
    groups.push({group:shard.name,replicas:physicalReplicas(shard.replicas)});
  }
  return {source,groups};
}

function connections(model, inflight) {
  const pools=model.groups.reduce((n,g)=>n+g.replicas,0);
  if (pools && inflight!==null && inflight<pools) throw Error('ClickHouse slot quota below physical replica count');
  const perPool=pools ? (inflight===null ? 6 : Math.min(6, Math.floor(inflight/pools))) : 0;
  return {physical_pools:pools,max_open_per_pool:perPool,max_open:pools*perPool};
}

function splitShare(architecture, role, id, total) {
  const slots=architecture.replica_slots?.[role]??[];
  if (!slots.length) {
    if (id!==role+'-1' || (architecture.replica_id && architecture.replica_id!==id)) throw Error('Runtime architecture does not register slot '+id);
    return total;
  }
  const index=slots.indexOf(id);
  if (index<0 || (architecture.replica_id && architecture.replica_id!==id)) throw Error('Runtime architecture does not register slot '+id);
  if (total<slots.length) throw Error('Budget below slot count for '+role);
  return Math.floor(total/slots.length) + Number(index<total%slots.length);
}

function mergeConfig(...configs) {
  const merged=Object.assign({},...configs);
  for (const key of ['display','derived','files','audit']) {
    merged[key]=Object.assign({},...configs.map(config=>config?.[key]));
  }
  return merged;
}

function workload(role, limit, config, dir) {
  const fg=()=>whole(config.derived.foreground_workers??1,1,16,'derived foreground workers');
  const hist=()=>whole(config.derived.historical_workers??1,1,16,'derived historical workers');
  const lanes={};
  if (role==='parser') {
    if (!dir) return {};
    const match=readFileSync(join(dir,'runtime.env'),'utf8').match(/^INFRAPRO_PARSER_CONCURRENCY=([0-9]+)$/m);
    lanes.parser_executions=whole(match?.[1],1,4,'parser concurrency');
  }
  if (['ingest-worker','monitoring-worker','parsing-dispatch-worker'].includes(role)) lanes.parser_shared={global:4,automatic_per_tenant:2,manual_reserved:1};
  if (['monitoring-publish-worker','monitoring-incremental-worker','monitoring-worker'].includes(role)) lanes.foreground_jobs_per_owner=fg();
  if (['monitoring-publish-worker','monitoring-incremental-worker','monitoring-history-worker','monitoring-read-worker','monitoring-worker'].includes(role)) lanes.historical_jobs_per_owner=hist();
  if (['monitoring-read-worker','monitoring-worker'].includes(role)) lanes.display_read_jobs_shared=hist();
  if (role==='monitoring-incremental-worker') lanes.inline_trial_jobs_shared=1;
  if (role==='files-checker') lanes.local_checks=1;
  if (role==='files-worker') lanes.file_preparation=whole(config.files.workers??2,1,32,'file workers');
  if (role==='audit-worker') lanes.reconciliation_and_export=whole(config.audit.workers??2,1,8,'audit workers');
  if (['spatial-worker','display-package-worker'].includes(role)) lanes.claimed_jobs_per_process=1;
  if (['notification-worker','task-index-worker'].includes(role)) lanes.fallback_scans_per_process=1;
  if (['ingest-custody-worker','ingest-worker'].includes(role)) {
    const intake=dir && existsSync(join(dir,'ingestion.json')) ? json(join(dir,'ingestion.json')) : null;
    if (intake) lanes.relay_pulls=4*(intake.relays??[]).length;
    lanes.relay_confirmations=2;
  }
  if (role==='event-relay') {
    const event=dir && existsSync(join(dir,'events.json')) ? json(join(dir,'events.json')) : null;
    if (event) lanes.broker_publications=event.mode==='jetstream' ?
      Math.min(whole(event.work_concurrency??4,1,32,'event work concurrency'),whole(event.batch??32,1,128,'event batch')) : 0;
  }
  if (role==='replica-proxy') lanes.tcp_sessions_all_listeners=limit;
  return lanes;
}

function mountedConfig(path, overrides) {
  if (!existsSync(path)) return {};
  try { return json(path); }
  catch (cause) {
    // A resolved settings file also accommodates YAML anchors and external
    // environment overrides. Never partially parse an arbitrary YAML document.
    if (overrides) return {};
    throw new Error('Supply --runtime-config with resolved settings for installed YAML', {cause});
  }
}

function readRuntimeOptions(path, slots) {
  if (!path) return {common:{},overrides:{},original:{},source:'mounted-runtime'};
  const raw=json(path), scoped=['defaults','slots','schema58'].some(key=>Object.hasOwn(raw,key));
  const common=scoped ? raw.defaults??{} : raw, overrides=scoped ? raw.slots??{} : {};
  if (Object.keys(overrides).some(id=>!slots.has(id))) throw Error('Runtime settings refer to an unknown deployment slot');
  const original=scoped ? raw.schema58??{} : {};
  if (Object.keys(original).some(role=>!originalStorageRoles.includes(role))) throw Error('Unknown schema 58 storage process');
  return {common,overrides,original,source:'resolved-runtime'};
}

// Apply only values present in the selected per-process environment. Process
// env wins over YAML, as in the Go config loader. Other slots cannot inherit
// secrets from this reader and endpoint strings never enter the published table.
function envSettings(env) {
  const config={};
  for (const key of ['monitoring_clickhouse_replicas','monitoring_clickhouse_database','monitoring_physical_shards']) {
    const value=env['INFRAPRO_'+key.toUpperCase()];
    if (value!==undefined) config[key]=value;
  }
  for (const [group,fields] of Object.entries({derived:['foreground_workers','historical_workers'],files:['workers'],audit:['workers'],display:['pool_size']})) {
    for (const field of fields) {
      const value=env['INFRAPRO_'+group.toUpperCase()+'_'+field.toUpperCase()];
      if (value!==undefined) (config[group]??={})[field]=value;
    }
  }
  return config;
}

function slotLimits(instance, dir, config, model) {
  const {role,id}=instance,spec=inventory.roles[role];
  const arch=dir && existsSync(join(dir,'architecture.json')) ? json(join(dir,'architecture.json')) : null;
  if (!arch && spec.module==='services/api') throw Error('Missing mounted architecture for '+id);
  const share=total=>splitShare(arch,role,id,total);
  const configured=key=>whole(arch.max_inflight?.[key]??32,1,16384,key+' in-flight limit');
  const inflight=arch ? share(configured(role)) : instance.maxInflight;
  const needsCH=spec.dependencies.includes('ch') || role==='display-runtime' && arch && !arch.split_display;
  let pg=arch && spec.pgConnections ? share(whole(arch.pg_connections?.[role]??12,1,256,role+' database pool')) : instance.pgConnections;
  const http=role==='api' ? Math.min(inflight,httpOwners.reduce((sum,owner)=>sum+share(configured(owner)),0)) :
    privateHTTP.has(role) ? inflight : null;
  const tasks=workload(role,inflight,config,dir);
  if (role==='ingest-relay') {
    const file=existsSync(join(dir,'relay.json'))?'relay.json':'relay.example.json',relay=json(join(dir,file));
    pg=whole(new URL(relay.database_url).searchParams.get('pool_max_conns'),2,65536,'spool pool');
    tasks.device_connections=whole(relay.max_connections,1,10000,'relay max connections');
  }
  if (pg!==instance.pgConnections) throw Error('Database pool and provisioned login budget differ for '+id);
  return {id,role,mode:spec.mode,placement:spec.placement,process_resources:processCapacity(instance),slot_max_inflight:inflight,pg_connections:pg,
    clickhouse:connections(needsCH?model:{groups:[]},inflight),http_inflight:http,work_limits:tasks,
    ...(needsCH ? {clickhouse_topology:model} : {})};
}

export function compareQuotaReport(slot, report) {
  if (report.replica_id!==slot.id || report.phase!=='ready' || !report.process?.pid || !report.process?.boot_id)
    throw Error('Expected a ready process report for slot '+slot.id);
  const metrics=new Map((report.metrics??[]).map(m=>[m.key,m.value]));
  const expected={'quota.pg_connections':slot.pg_connections,'quota.ch_connections':slot.clickhouse.max_open,
    'quota.http_inflight':slot.http_inflight,'quota.parser_slots':slot.work_limits.parser_executions??slot.work_limits.parser_shared?.global};
  const observed={};
  for (const [key,cap] of Object.entries(expected)) {
    if (cap && !metrics.has(key)) throw Error(slot.id+' missing '+key);
    if (metrics.has(key)) {
      if (metrics.get(key)!==(cap??0)) throw Error(slot.id+' mismatched '+key+' (expected '+(cap??0)+')');
      observed[key]=metrics.get(key);
    }
  }
  if (metrics.has('quota.s3_inflight')) {
    if (metrics.get('quota.s3_inflight')!==slot.slot_max_inflight) throw Error(slot.id+' mismatched quota.s3_inflight');
    observed['quota.s3_inflight']=metrics.get('quota.s3_inflight');
  }
  return {boot_id:report.process.boot_id,pid:report.process.pid,emitted_at:report.emitted_at,observed};
}

function aggregate(slots) {
  return {slots:slots.length,pg_connections:slots.reduce((n,s)=>n+s.pg_connections,0),
    ch_connections:slots.reduce((n,s)=>n+s.clickhouse.max_open,0)};
}

export function resourceLimits(directory, options={}) {
  const layout=json(join(directory,'layout.json')), instances=Object.values(layout.instances).flat();
  const ids=new Set(instances.map(s=>s.id)), runtime=readRuntimeOptions(options['runtime-config'],ids);
  const overrideEnv=envSettings(options.environment??{}), explicit=Boolean(options['runtime-config']);
  const slots=instances.map(instance=>{
    const dir=instance.directory ? join(directory,instance.directory) : null;
    const fromFile=dir ? mountedConfig(join(dir,'runtime.yaml'),explicit) : {};
    const config=mergeConfig(fromFile,runtime.common,overrideEnv,runtime.overrides[instance.id]);
    return slotLimits(instance,dir,config,topology(config,explicit,runtime.source));
  });
  const originalConfig=mergeConfig(runtime.common,overrideEnv);
  const originalRoles=originalStorageRoles.map(role=>{
    const config=mergeConfig(originalConfig,runtime.original[role]);
    return {role,pg_connections:role==='display-runtime'?whole(config.display.pool_size??8,1,32,'S0 display pool'):12,
      clickhouse:connections(topology(config,explicit,runtime.source),null),architecture_http_gate:null};
  });
  let checked=0;
  const reports=options['report-map'] ? json(options['report-map']) : {};
  if (Object.keys(reports).some(id=>!ids.has(id))) throw Error('Report map refers to an unknown deployment slot');
  for (const slot of slots) {
    const label=instances.find(i=>i.id===slot.id).label;
    const root=options['reports-directory'];
    const candidates=root ? [join(root,label+'.json'),join(root,label,'process.json'),join(root,'infrapro-'+label,'process.json')] : [];
    const report=reports[slot.id]??candidates.find(existsSync);
    if (!report || slot.role==='replica-proxy') continue;
    slot.process_report=compareQuotaReport(slot,json(report));
    checked++;
  }
  if ((options['reports-directory'] || options['report-map']) && !checked) throw Error('No process quota reports matched');
  const scope=layout.process_resources??{roles:[],schema58:{}};
  return {version:1,deployment_mode:layout.mode,slots,checked_reports:checked,
    selected_process_profiles:{roles:scope.roles,split:processTotals(instances.filter(s=>scope.roles.includes(s.role))),
      schema58:processTotals(Object.values(scope.schema58)),schema58_roles:Object.fromEntries(Object.entries(scope.schema58).map(([r,i])=>[r,processCapacity(i)]))},
    totals:{split_application:aggregate(slots.filter(s=>s.placement==='application' && ['split','both'].includes(s.mode))),
      same_schema_compatibility_additions:aggregate(slots.filter(s=>['compat','standby'].includes(s.mode))),
      independent_edge:aggregate(slots.filter(s=>s.placement==='edge'))},
    separately_supervised:Object.entries(inventory.roles).filter(([role])=>
      !layout.instances[role] && role!=='replica-proxy' && role!=='infrapro').map(([role,spec])=>({
        role,mode:spec.mode,placement:spec.placement,declared_max_inflight:spec.maxInflight,
        pg_connections:spec.pgConnections,clickhouse:connections({groups:[]},null),architecture_http_gate:null})),
    original_schema58_storage_processes:originalRoles,
    manual_storage_command:{role:'infrapro',clickhouse:connections(topology(originalConfig,explicit,runtime.source),null)}};
}

export function writeResourceLimits(directory, options={}) {
  const result=resourceLimits(directory,options);
  writeRuntimeJSON(join(directory,'resource-limits.json'),result);
  return result;
}
