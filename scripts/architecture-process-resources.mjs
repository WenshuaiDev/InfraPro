import { join } from 'node:path';
import { inventory } from './architecture-runtime.mjs';

const fields=new Set(['cpu','memory_mib','go_threads','go_memory_mib']);
function share(total,index,count) {
  return Math.floor(total/count)+Number(index<total%count);
}

function applyBudget(role, slots, value) {
  if (!value || typeof value!=='object' || Array.isArray(value) || Object.keys(value).some(key=>!fields.has(key)))
    throw Error('Invalid process resource profile for '+role);
  const cpu=value.cpu??inventory.roles[role].cpu;
  const cpuUnits=Math.round(cpu*10000);
  if (typeof cpu!=='number' || !Number.isSafeInteger(cpuUnits) || Math.abs(cpu*10000-cpuUnits)>1e-7 || cpuUnits<slots.length)
    throw Error('Invalid CPU quota for '+role);
  const memory=value.memory_mib??inventory.roles[role].memoryMiB;
  const threads=value.go_threads, soft=value.go_memory_mib;
  for (const n of [memory,threads,soft]) {
    if (n!==undefined && (!Number.isSafeInteger(n) || n<slots.length)) throw Error('Resource total below slot count for '+role);
  }
  if (soft>memory) throw Error('Go memory target exceeds the process memory limit for '+role);
  slots.forEach((slot,index)=>{
    slot.cpu=share(cpuUnits,index,slots.length)/10000;
    slot.memoryMiB=share(memory,index,slots.length);
    if (threads!==undefined) slot.goThreads=share(threads,index,slots.length);
    slot.goMemoryMiB=soft===undefined ? Math.max(1,Math.floor(slot.memoryMiB*0.8)) : share(soft,index,slots.length);
  });
}

export function processProfiles(instances,profile={}) {
  const scoped=Object.hasOwn(profile,'roles') || Object.hasOwn(profile,'schema58');
  if (scoped && Object.keys(profile).some(k=>!['roles','schema58'].includes(k))) throw Error('Unknown process resource profile group');
  const roles=scoped ? profile.roles??{} : profile;
  for (const [role,settings] of Object.entries(roles)) {
    if (!instances[role]) throw Error('Unregistered process resource role '+role);
    applyBudget(role,instances[role],settings);
  }
  const original={};
  for (const [role,settings] of Object.entries(scoped?profile.schema58??{}:{})) {
    const spec=inventory.roles[role];
    if (!spec || !['both','compat'].includes(spec.mode) || !['application','edge'].includes(spec.placement))
      throw Error('Unknown original process resource role '+role);
    const old={role};
    applyBudget(role,[old],settings);
    original[role]=old;
  }
  return {roles:Object.keys(roles),schema58:original};
}

// Preserve inherited runtime choices for installations without an explicit
// process profile. Explicit settings take precedence in every local launcher.
export function processEnvironment(slot, inherited={}, withDefaultMemory=false) {
  const env={...inherited};
  if (slot.goThreads) env.GOMAXPROCS=String(slot.goThreads);
  if (slot.goMemoryMiB) env.GOMEMLIMIT=slot.goMemoryMiB+'MiB';
  else if (withDefaultMemory && slot.memoryMiB) env.GOMEMLIMIT??=Math.floor(slot.memoryMiB*0.8)+'MiB';
  return env;
}

export function renderOriginalProcessLimits(directory, profiles, save) {
  const services={};
  for (const [role,slot] of Object.entries(profiles.schema58)) {
    const env=processEnvironment(slot);
    const lines=Object.entries(env).map(([k,v])=>k+'='+v);
    const target='infrapro-'+role+'.service.d';
    save(join(directory,'schema58/systemd',target,'30-resource-limits.conf'),
      ['[Service]','CPUQuota='+(slot.cpu*100).toFixed(2)+'%','MemoryMax='+slot.memoryMiB+'M',...lines.map(l=>'Environment='+l),''].join('\n'),0o644);
    save(join(directory,'schema58/environment',role+'.env'),lines.join('\n')+'\n',0o644);
    services[role]={cpus:slot.cpu,mem_limit:slot.memoryMiB+'m',environment:env};
  }
  if (Object.keys(services).length) save(join(directory,'schema58/compose-limits.json'),{services},0o644);
}

export function processCapacity(slot) {
  return {cpu_cores:slot.cpu??null,memory_mib:slot.memoryMiB??null,
    go_threads:slot.goThreads??null,go_memory_mib:slot.goMemoryMiB??(slot.memoryMiB?Math.floor(slot.memoryMiB*0.8):null)};
}

export function processTotals(slots) {
  const capacity=slots.map(processCapacity);
  return {processes:slots.length,cpu_cores:Math.round(capacity.reduce((n,c)=>n+c.cpu_cores,0)*10000)/10000,
    memory_mib:capacity.reduce((n,c)=>n+c.memory_mib,0),
    go_threads:capacity.every(c=>c.go_threads!==null)?capacity.reduce((n,c)=>n+c.go_threads,0):null,
    go_memory_mib:capacity.reduce((n,c)=>n+c.go_memory_mib,0)};
}
