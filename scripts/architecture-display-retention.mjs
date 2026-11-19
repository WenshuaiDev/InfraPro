import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, readdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { fileLease, read, runDirectory, write } from './architecture-lab.mjs';
import { retainDisplayStorage } from './architecture-display-lab.mjs';

const docker = (args) => execFileSync('docker', args, { encoding: 'utf8' }).trim();

// The source snapshot and the prepared first scene are excluded. Archived
// trials keep every data file and retain their exact table names for ATTACH.
export function parkPastDisplayTrials(dir, exceptWorkspace) {
  const workspaces = join(dir, 'private/display-load');
  const endpoints = read(join(workspaces,'infra/private/http-storage/endpoints.json'));
  const owner = endpoints.chContainer;
  assert.match(owner,/^arc-http-storage-[a-f0-9]{12}-ch1-1$/);
  const sql = (query) => docker(['exec',owner,'clickhouse-client','--query',query]);
  const rows = [], at = new Date().toISOString();
  let roundTrip;
  for (const id of readdirSync(workspaces).filter(id => /^[a-f0-9]{12}$/.test(id)).sort()) {
    const workspace = join(workspaces,id), config = join(workspace,'runtime.json'), cohort = join(workspace,'cohort.json');
    if (workspace === exceptWorkspace) continue;
    if (!existsSync(config)) continue;
    const cfg = read(config), db = cfg.monitoring_clickhouse_database;
    if (db !== 'arc_load_'+id || cfg.monitoring_clickhouse_replicas !== endpoints.ch || (existsSync(cohort) && read(cohort).phase === 'prepared')) continue;
    const attached = Number(sql("SELECT count() FROM system.tables WHERE database='"+db+"'"));
    if (!attached) continue;
    const physical = () => sql("SELECT count(), sum(rows), groupBitXor(cityHash64(table, name, rows, bytes_on_disk)) FROM system.parts WHERE active AND database='"+db+"'");
    const before = physical(), stack = {workspace,chName:db,storage:{clickhouse:owner}};
    const receipt = retainDisplayStorage(stack);
    assert.equal(sql("SELECT count() FROM system.tables WHERE database='"+db+"'"),'0');
    if (!roundTrip) {
      for (const name of receipt.tables) sql('ATTACH TABLE '+db+'.'+name);
      const after = physical();
      assert.equal(after,before,'Reattaching a completed trial must restore the same active data parts');
      roundTrip = {database:db,before,after};
      retainDisplayStorage(stack);
    }
    rows.push({database:db,tables:receipt.tables,activeDataParts:before});
  }
  const resources = displayHostResources(dir);
  const result={at,finished:new Date().toISOString(),rows,roundTrip,resources};
  write(join(dir,'load/display/retention',at.replaceAll(':','-')+'.json'),result);
  return result;
}

export function displayHostResources(dir) {
  const pg = read(join(dir,'load/display/postgres.json')).container;
  const owner=read(join(dir,'private/display-load/infra/private/http-storage/endpoints.json')).chContainer;
  const names = [pg,owner,owner.replace('-ch1-1','-ch2-1'),owner.replace('-ch1-1','-keeper-1'),owner.replace('-ch1-1','-s3-1'),'infrapro-account-tests-redis-1'];
  return names.map(name => {
    const state = JSON.parse(docker(['inspect','--format','{{json .}}',name]));
    assert.equal(state.State.Running,true,name);
    return {name,image:state.Image,cpuNano:state.HostConfig.NanoCpus,memoryBytes:state.HostConfig.Memory,sharedMemoryBytes:state.HostConfig.ShmSize};
  });
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const dir=runDirectory(process.argv[2]);
  const unlock=await fileLease(join(dir,'load/display/campaign.lock'),{wait:false});
  try {
    const result=parkPastDisplayTrials(dir);
    console.log('Retained tables from',result.rows.length,'completed display databases');
  } finally {await unlock();}
}
