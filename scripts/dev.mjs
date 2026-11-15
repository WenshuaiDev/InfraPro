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
  const binary = join(root, 'build', 'dev', 'api');
  execFileSync('go', ['build', '-o', binary, './services/api/cmd/api'], { cwd: root, stdio: 'inherit' });
  if(!process.argv.includes('--no-files')){
    const dir=process.env.INFRAPRO_FILES_TEMP_DIRECTORY??join(root,'.local/files/tmp');mkdirSync(dir,{recursive:true,mode:0o700});
    process.env.INFRAPRO_FILES_TEMP_DIRECTORY=dir;process.env.INFRAPRO_FILES_CHECKER_SOCKET??=join(dir,'checker.sock');
    for(const name of ['files-checker','files-worker'])execFileSync('go',['build','-o',join(root,'build/dev',name),`./services/api/cmd/${name}`],{cwd:root,stdio:'inherit'});
    start('files checker',join(root,'build/dev/files-checker'),['--directory',dir,'--socket',process.env.INFRAPRO_FILES_CHECKER_SOCKET],{PATH:process.env.PATH,GOMEMLIMIT:'256MiB',GOMAXPROCS:'2',TZ:'UTC'});
    start('files worker',join(root,'build/dev/files-worker'),['--allow-unconfigured']);
    console.log('[dev] Files checker and worker started; files check verifies socket, S3 and heartbeat. macOS parser has no Linux OS sandbox.');
  }
  start('api', binary, []);
  if (!process.argv.includes('--no-audit')) { const worker=join(root,'build','dev','audit-worker');execFileSync('go',['build','-o',worker,'./services/api/cmd/audit-worker'],{cwd:root,stdio:'inherit'});start('audit worker',worker,[]); }
  if (!process.argv.includes('--no-tasks')) { const worker=join(root,'build','dev','task-index-worker');execFileSync('go',['build','-o',worker,'./services/api/cmd/task-index-worker'],{cwd:root,stdio:'inherit'});start('task index worker',worker,[]); }
  if (!process.argv.includes('--no-notifications')) { const worker=join(root,'build','dev','notification-worker');execFileSync('go',['build','-o',worker,'./services/api/cmd/notification-worker'],{cwd:root,stdio:'inherit'});start('notification worker',worker,[]); }
  if(process.argv.includes('--spatial')){const worker=join(root,'build','dev','spatial-worker');execFileSync('go',['build','-o',worker,'./services/api/cmd/spatial-worker'],{cwd:root,stdio:'inherit'});start('spatial worker',worker,[]);}
  if (!apiOnly) {
    start('console', 'pnpm', ['--filter', '@infrapro/console', preview ? 'preview' : 'dev']);
  }
  console.log('[dev] Press Ctrl+C to stop all processes started by this command.');
} catch (error) {
  console.error(error.message);
  stop(1);
}
