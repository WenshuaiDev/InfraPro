import { execFileSync } from 'node:child_process';

// Read only the numeric process fields. Command lines, environment variables,
// account names and unrelated processes never enter the verification artifact.
export function sampleProcessTree(rootPID) {
  if (!rootPID) return { rootPID: null, processes: [], summedRSSBytes: 0 };
  try {
    const rows = execFileSync('ps', ['-axo', 'pid=,ppid=,pcpu=,rss='], { encoding: 'utf8' })
      .trim().split('\n').map(line => line.trim().split(/\s+/).map(Number))
      .filter(row => row.length === 4 && row.every(Number.isFinite));
    const selected = new Set([rootPID]);
    let changed = true;
    while (changed) {
      changed = false;
      for (const [pid, parent] of rows) if (selected.has(parent) && !selected.has(pid)) {
        selected.add(pid); changed = true;
      }
    }
    const processes = rows.filter(([pid]) => selected.has(pid)).map(([pid, parentPID, lifetimeCPUPercent, rssKiB]) => ({ pid, parentPID, lifetimeCPUPercent, rssBytes: rssKiB * 1024 }));
    return { rootPID, processes, summedRSSBytes: processes.reduce((sum, p) => sum + p.rssBytes, 0) };
  } catch (error) { return { rootPID, error: error.message }; }
}
