import { chmodSync, mkdirSync, unlinkSync } from 'node:fs';
import { join } from 'node:path';
import { root, readJSON, privateJSON, command, saveLab, startProcess, stopProcess, resumeProcess, processAlive, compose, ownedContainers, checkContainerOwnership, sleep, waitHTTP } from './diagnostics-lab.mjs';
export const faultCases=['business-offline','postgres-offline','worker-stop','worker-loop-failure','observer-disconnect','stale-replay','container-restart','permission-denied','disk-pressure','store-unwritable','report-corrupt','credential-revoke','clock-skew'];
function container(lab,replica){const ids=ownedContainers(lab).filter(id=>JSON.parse(command('docker',['inspect','--format','{{json .Config.Labels}}',id]))['io.infrapro.replica']===replica);if(ids.length!==1)throw Error('Owned container selector is not unique');checkContainerOwnership(lab,ids[0]);return ids[0]}
export async function rebind(lab,node,bindings){
 const manifest=readJSON(lab.manifest_file),n=manifest.nodes.find(n=>n.id===node),assignment=String(BigInt(n.assignment_revision)+1n);n.assignment_revision=assignment;manifest.revision=String(BigInt(manifest.revision)+1n);
 const cfg=readJSON(lab.server_file),keys=readJSON(cfg.node_keys_file);keys.revision=String(BigInt(keys.revision)+1n);for(const k of keys.keys)if(k.node_id===node&&k.enabled)k.assignment_revision=assignment;
 bindings.assignment_revision=assignment;const observerPath=join(lab.directory,node,'observer.json'),observer=readJSON(observerPath);observer.assignment_revision=assignment;
 privateJSON(join(lab.directory,node,'bindings.json'),bindings);privateJSON(observerPath,observer);privateJSON(cfg.node_keys_file,keys);privateJSON(lab.manifest_file,manifest);await sleep(5500);
}
async function restartServer(lab){await stopProcess(lab,'server');await resumeProcess(lab,'server');await waitHTTP(lab.origin)}
export async function injectFault(lab,name){
 if(!faultCases.includes(name))throw Error('Unknown fixed diagnostics fault');if(lab.faults.length)throw Error('Recover the current lab fault before another fault');
 if(!lab.business&&['business-offline','postgres-offline','worker-stop','worker-loop-failure','container-restart','permission-denied','report-corrupt'].includes(name))throw Error('This fault needs the full isolated business lab');
 lab.faults=[name];lab.recovery={};saveLab(lab);
 switch(name){
 case 'business-offline':await stopProcess(lab,'api');for(const r of ['web-1','postgres-1'])command('docker',['stop','--time','5',container(lab,r)]);break;
 case 'postgres-offline':case 'worker-loop-failure':command('docker',['stop','--time','5',container(lab,'postgres-1')]);break;
 case 'worker-stop':await stopProcess(lab,'notification-worker');break;
 case 'observer-disconnect':await stopProcess(lab,'native-c');break;
 case 'container-restart':command('docker',['restart','--time','5',container(lab,'clickhouse-2')]);break;
 case 'permission-denied':case 'report-corrupt':{
  const path=join(lab.directory,'native-a/bindings.json'),b=readJSON(path);lab.recovery.bindings=b;lab.recovery.binding_node='native-a';saveLab(lab);
  const file=join(lab.directory,'reports',name+'.json');privateJSON(file,{invalid:'isolated lab fault'});if(name==='permission-denied')chmodSync(file,0o000);
  const changed=structuredClone(b);changed.bindings.find(b=>b.id==='notification-worker-report').report_file=file;await rebind(lab,'native-a',changed);break;
 }
 case 'disk-pressure':{
  const cfg=readJSON(lab.server_file);lab.recovery.server=cfg;saveLab(lab);const changed=structuredClone(cfg);changed.resource_hints.enabled=true;changed.resource_hints.disk_available_bytes='999999999999999';privateJSON(lab.server_file,changed);await restartServer(lab);break;
 }
 case 'store-unwritable':{
  const cfg=readJSON(lab.server_file);lab.recovery.server=cfg;const dir=join(lab.directory,'fault-read-only-state');mkdirSync(dir,{recursive:true,mode:0o700});chmodSync(dir,0o500);lab.recovery.read_only_directory=dir;saveLab(lab);privateJSON(lab.server_file,{...cfg,state_dir:dir});await restartServer(lab);break;
 }
 case 'credential-revoke':command(lab.cli,['diagnostics','auth','set-enabled','--server',lab.server_file,'--username','operator','--enabled','false']);command(lab.cli,['diagnostics','observer-key','revoke','--server',lab.server_file,'--node-id','native-c']);break;
 case 'clock-skew':case 'stale-replay':await stopProcess(lab,'native-c');await startProcess(lab,'fault-publisher',process.execPath,[join(root,'scripts/fixtures/diagnostics-fault-publisher.mjs'),join(lab.directory,'lab.json'),name]);break;
 }
 saveLab(lab);return {fault:name,run_id:lab.run_id,origin:lab.origin,expected:expectations[name]};
}
export async function recoverLab(lab){
 const names=[...lab.faults];await stopProcess(lab,'fault-publisher');
 if(lab.recovery?.server){if(lab.recovery.read_only_directory)chmodSync(lab.recovery.read_only_directory,0o700);privateJSON(lab.server_file,lab.recovery.server);await restartServer(lab)}
 if(lab.recovery?.bindings)await rebind(lab,lab.recovery.binding_node,lab.recovery.bindings);
 if(names.includes('credential-revoke')){command(lab.cli,['diagnostics','auth','set-enabled','--server',lab.server_file,'--username','operator','--enabled','true']);const key=join(lab.directory,'native-c/key.json');unlinkSync(key);command(lab.cli,['diagnostics','observer-key','rotate','--server',lab.server_file,'--node-id','native-c','--output-file',key]);}
 if(lab.compose_project){for(const id of ownedContainers(lab))checkContainerOwnership(lab,id);compose(lab,['up','-d','--wait','--wait-timeout','120']);}
 for(const name of Object.keys(lab.processes)){if(name==='fault-publisher')continue;if(!processAlive(lab,name))await resumeProcess(lab,name)}
 lab.faults=[];lab.recovery={};saveLab(lab);await sleep(6500);return {run_id:lab.run_id,recovered:names,origin:lab.origin};
}
export const expectations={
 'business-offline':'仅本 lab web、API、PG 停止；诊断可新登录，目标故障作为数据返回。',
 'postgres-offline':'PG 容器停止，固定 SQL 探针失败；诊断不依赖 PG。',
 'worker-stop':'notification-worker 精确进程缺失；与存活但循环失败分开。',
 'worker-loop-failure':'本 lab PG 停止，notification/task-index 等仍存活的循环报告依赖错误。',
 'observer-disconnect':'20 秒后 native-c 离线、旧数据过期；其他节点继续更新。',
 'stale-replay':'独立故障发布进程重发同一完整帧；duplicate 不刷新时间或在线状态。',
 'container-restart':'第二个 ClickHouse 容器重启，启动代次变化，副本分母仍为 2。',
 'permission-denied':'只将本 lab 检查指向 000 权限文件；进程仍存活，报告检查显示无权限。',
 'disk-pressure':'仅临时调高本 lab 磁盘提示阈值，使用真实采样触发提示；不写满磁盘。',
 'store-unwritable':'临时使用不可写的独立状态目录；原库完整保留，远端上报拒绝，本地标 volatile。',
 'report-corrupt':'只读本 lab 固定损坏报告 fixture，显示报告无效，原业务报告不被修改。',
 'credential-revoke':'本 lab 诊断账号与 native-c key 撤销；旧会话失效，recover 轮换节点 key。',
 'clock-skew':'单独故障发布进程发送未来一分钟采样，显示时间不可信。',
};
