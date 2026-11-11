"""Feed the real local maintenance CLI through a no-echo PTY; never print secrets."""
import json
import os
from pathlib import Path
import pty
import select
import subprocess
import termios
import time

ROOT = Path(__file__).resolve().parents[2]
DIRECTORY = ROOT / '.artifacts/project-resource-acceptance'
if os.environ.get('INFRAPRO_REDIS_KEY_PREFIX') != 'infrapro:project-resource-acceptance20261111:identity:v1:':
    raise RuntimeError('Refusing to bootstrap outside the acceptance environment')
account = json.loads((DIRECTORY / 'accounts.local.json').read_text())['platform']
marker = DIRECTORY / 'bootstrap.complete'
if marker.exists():
    print('Acceptance bootstrap already completed; existing credentials retained.')
    raise SystemExit(0)
master, slave = pty.openpty()
proc = subprocess.Popen([str(ROOT / 'build/project-resource-acceptance/infrapro'), 'account', 'bootstrap'], stdin=slave, stdout=slave, stderr=slave, close_fds=True)
os.close(slave)
prompts = [('用户名：', account['username']), ('显示名称（', account['display_name']), ('密码（8', account['password']), ('再次输入密码：', account['password'])]
transcript, index = '', 0
deadline = time.monotonic() + 45
try:
    while time.monotonic() < deadline:
        if select.select([master], [], [], 0.1)[0]:
            try:
                data = os.read(master, 65536)
            except OSError:
                break
            if not data:
                break
            transcript += data.decode('utf-8', 'replace')
            if index < len(prompts) and prompts[index][0] in transcript:
                if index >= 2:
                    # The prompt is printed just before Go disables terminal echo.
                    while termios.tcgetattr(master)[3] & termios.ECHO:
                        if time.monotonic() >= deadline:
                            raise RuntimeError('Password terminal did not disable echo')
                        time.sleep(0.01)
                os.write(master, (prompts[index][1] + '\n').encode())
                index += 1
        if proc.poll() is not None:
            break
finally:
    if proc.poll() is None:
        proc.terminate()
    proc.wait(timeout=5)
    os.close(master)
if account['password'] in transcript or proc.returncode or '首个账号创建成功' not in transcript:
    safe = transcript.replace(account['password'], '[redacted]')
    raise RuntimeError(f'Acceptance bootstrap failed (prompt {index}, exit {proc.returncode}): {safe}')
marker.write_text('Created through the existing interactive maintenance CLI.\n')
marker.chmod(0o600)
print('Acceptance platform administrator created; password was not echoed.')
