import { processEnvironment } from './architecture-process-resources.mjs';
import { deploymentDirectory, roleEnvironment, enabledSplitRoles, serviceInstances,localReplicaProxy } from './architecture-runtime.mjs';
import { spawn, execFileSync } from 'node:child_process';
import { existsSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));
if (existsSync(join(root, '.env'))) process.loadEnvFile(join(root, '.env'));
const apiOnly = process.argv.includes('--api-only');
const preview = process.argv.includes('--preview');
const children = new Set();
let stopping = false;

function stop(code) {
  if (stopping) return;
  stopping = true;
  const active = [...children];
  const signal = (child, name) => {
    if (!child.pid) return;
    try { process.kill(-child.pid, name); } catch (error) {
      if (error.code !== 'ESRCH') console.error(error);
    }
  };
  const completions = active.map((child) => new Promise((resolve) => child.once('close', resolve)));
  active.forEach((child) => signal(child, 'SIGTERM'));
  const timeout = setTimeout(() => active.forEach((child) => signal(child, 'SIGKILL')), 8000);
  Promise.all(completions).then(() => { clearTimeout(timeout); process.exit(code); });
}

function start(name, command, args, environment=process.env) {
  console.log(`[dev] starting ${name}`);
  const child = spawn(command, args, { cwd: root, stdio: 'inherit', detached: true, env: environment });
  children.add(child);
  child.on('error', (error) => { console.error(`[dev] ${name}: ${error.message}`); stop(1); });
  child.on('close', () => children.delete(child));
  child.on('exit', (code, signal) => {
    if (!stopping) {
      console.error(`[dev] ${name} stopped (${signal ?? code}); stopping the other processes.`);
      stop(code || 1);
    }
  });
}

process.on('SIGINT', () => stop(0));
process.on('SIGTERM', () => stop(0));

try {
  if (preview && !apiOnly) {
    if (!existsSync(join(root, 'apps', 'console', 'dist', 'index.html'))) {
      throw new Error('Build the application first with make build.');
    }
  }
  mkdirSync(join(root, 'build', 'dev'), { recursive: true });
  const architecture = deploymentDirectory(process.env);
  const startRole=(role,file,args=[],independent)=>{
    for(const instance of serviceInstances(role,architecture)){
      const settings=independent?{...independent}:roleEnvironment(role,process.env,architecture,instance);
      if(architecture){
        const reports=join(architecture,'reports',instance.label);mkdirSync(reports,{recursive:true,mode:0o700});
        settings.INFRAPRO_DIAGNOSTICS_REPORT_FILE=join(reports,'process.json');
        settings.INFRAPRO_DIAGNOSTICS_REPLICA_ID=instance.id;
        settings.INFRAPRO_DIAGNOSTICS_INSTANCE_ID??='infrapro-development';
        settings.INFRAPRO_DIAGNOSTICS_NODE_ID=instance.diagnostic_node??settings.INFRAPRO_DIAGNOSTICS_NODE_ID??'local';
      }
      start(instance.label,file,args,processEnvironment(instance,settings));
    }
  };
  const proxy=localReplicaProxy(architecture,process.env);
  if(proxy)start('replica-proxy','python3',[join(root,'deploy/architecture/replica-proxy.py'),proxy],{PATH:process.env.PATH,INFRAPRO_DIAGNOSTICS_REPORT_FILE:join(architecture,'reports/replica-proxy/process.json')});
  const binary = join(root, 'build', 'dev', 'api');
  execFileSync('go', ['build', '-o', binary, './services/api/cmd/api'], { cwd: root, stdio: 'inherit' });
  if(!process.argv.includes('--no-files')){
    const dir=process.env.INFRAPRO_FILES_TEMP_DIRECTORY??join(root,'.local/files/tmp');mkdirSync(dir,{recursive:true,mode:0o700});
    process.env.INFRAPRO_FILES_TEMP_DIRECTORY=dir;process.env.INFRAPRO_FILES_CHECKER_SOCKET??=join(dir,'checker.sock');
    for(const name of ['files-checker','files-worker'])execFileSync('go',['build','-o',join(root,'build/dev',name),`./services/api/cmd/${name}`],{cwd:root,stdio:'inherit'});
    startRole('files-checker',join(root,'build/dev/files-checker'),['--directory',dir,'--socket',process.env.INFRAPRO_FILES_CHECKER_SOCKET],{PATH:process.env.PATH,GOMEMLIMIT:'256MiB',GOMAXPROCS:'2',TZ:'UTC'});
    startRole('files-worker',join(root,'build/dev/files-worker'),['--allow-unconfigured']);
    console.log('[dev] Files checker and worker started; files check verifies socket, S3 and heartbeat. macOS parser has no Linux OS sandbox.');
  }
  if (architecture) {
    const split = enabledSplitRoles(process.env, architecture);
    execFileSync('go', ['build', '-o', join(root,'build/dev/'), ...split.map(role => './services/api/cmd/' + role)], {cwd:root, stdio:'inherit'});
    for (const role of split) startRole(role, join(root,'build/dev',role));
  }
  startRole('api',binary);
  if (!process.argv.includes('--no-audit')) { const worker=join(root,'build','dev','audit-worker');execFileSync('go',['build','-o',worker,'./services/api/cmd/audit-worker'],{cwd:root,stdio:'inherit'});startRole('audit-worker',worker); }
  if (!process.argv.includes('--no-tasks')) { const worker=join(root,'build','dev','task-index-worker');execFileSync('go',['build','-o',worker,'./services/api/cmd/task-index-worker'],{cwd:root,stdio:'inherit'});startRole('task-index-worker',worker); }
  if (!process.argv.includes('--no-notifications')) { const worker=join(root,'build','dev','notification-worker');execFileSync('go',['build','-o',worker,'./services/api/cmd/notification-worker'],{cwd:root,stdio:'inherit'});startRole('notification-worker',worker); }
  if(process.argv.includes('--spatial')){const worker=join(root,'build','dev','spatial-worker');execFileSync('go',['build','-o',worker,'./services/api/cmd/spatial-worker'],{cwd:root,stdio:'inherit'});startRole('spatial-worker',worker);}
  if (!apiOnly) {
    start('console', 'pnpm', ['--filter', '@infrapro/console', preview ? 'preview' : 'dev']);
  }
  console.log('[dev] Press Ctrl+C to stop all processes started by this command.');
} catch (error) {
  console.error(error.message);
  stop(1);
}
