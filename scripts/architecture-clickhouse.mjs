import { inventory } from './architecture-runtime.mjs';

// The layout and results live in the application database. Runtime users can
// verify physical schemas and replication but cannot create or replace tables.
const metadata={
  tables:['database','name','engine','storage_policy','sorting_key','partition_key','create_table_query'],
  columns:['database','table','name','type','position'],
  replicas:['database','table','replica_name','zookeeper_path','is_readonly','is_session_expired','total_replicas','active_replicas'],
  storage_policies:['policy_name','volume_name','disks'],disks:['name','object_storage_type','cache_path'],
  filesystem_cache_settings:['cache_name','max_size'],
};
export function clickhouseProfile(role,database){
  if(!inventory.roles[role]?.dependencies.includes('ch')||role==='infrapro')return;
  if(!/^[A-Za-z_][A-Za-z0-9_]{0,62}$/.test(database))throw Error('Invalid ClickHouse monitoring database');
  const principal='infrapro_ch_'+database+'_'+role.replaceAll('-','_');
  const sql=['CREATE ROLE IF NOT EXISTS '+principal+';'];
  const grant=(action,table)=>sql.push('GRANT '+action+' ON '+table+' TO '+principal+';');
  for(const name of ['observations','monitoring_layout','derived_result_layout','derived_results_local'])grant('SELECT',database+'.'+name);
  for(const [table,fields] of Object.entries(metadata))grant('SELECT('+fields.map(f=>'\x60'+f+'\x60').join(',')+')','system.'+table);
  if(['monitoring-publish-worker','monitoring-worker'].includes(role)){
    grant('INSERT',database+'.observations');
    // Verification also purges already unselected calculation attempts.
    grant('ALTER DELETE',database+'.derived_results_local');
  }
  if(['monitoring-incremental-worker','monitoring-history-worker','monitoring-worker'].includes(role))
    grant('INSERT',database+'.derived_results_local');
  return {principal,sql:sql.join('\n')+'\n'};
}
