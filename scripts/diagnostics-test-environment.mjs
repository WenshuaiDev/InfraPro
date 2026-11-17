import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { startDiagnosticsLab, loadLab, artifacts } from './fixtures/diagnostics-lab.mjs';
let lab;if(existsSync(join(artifacts,'active.json'))){const previous=loadLab();if(previous.state==='running')lab=previous;}
if(!lab)lab=await startDiagnosticsLab({business:!process.argv.includes('--native-only')});
console.log(`独立诊断：${lab.origin}\n受限账号文件（0600）：${lab.accounts_file}\n本次运行：${lab.run_id}\n状态／故障／恢复／关闭：pnpm diagnostics:lab -- status|fault <case>|recover|down\n本地凭据不打印、不放入 URL。`);
