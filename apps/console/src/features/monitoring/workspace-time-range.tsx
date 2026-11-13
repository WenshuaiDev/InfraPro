import { useState } from 'react';
import type { MonitoringRange } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Field, MonitoringError } from './common';
import { formatNS, MAX_NS, MIN_NS, parseNS } from './format';

function localValue(value: string | null) {
  if (!value) return '';
  const ns = parseNS(value);
  const date = new Date(Number(ns < 0n ? (ns - 999_999n) / 1_000_000n : ns / 1_000_000n));
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 19);
}
export function MonitoringTimeRange({ range, earliest, latest, onChange }: { range: MonitoringRange; earliest?: string | null; latest?: string | null; onChange: (range: MonitoringRange) => void }) {
  const [from, setFrom] = useState(range.from ?? ''), [to, setTo] = useState(range.to ?? ''), [error, setError] = useState<Error | null>(null);
  const local = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const change = (value: string, set: (value: string) => void) => {
    if (!value) { set(''); return; }
    const date = new Date(value);
    if (Number.isFinite(date.getTime())) set(date.toISOString());
  };
  const safeLocal = (value: string) => { try { return localValue(value || null); } catch { return ''; } };
  return <form className="space-y-5" onSubmit={e => {
    e.preventDefault();
    try {
      const start = from ? parseNS(from) : MIN_NS, end = to ? parseNS(to) : MAX_NS;
      if (end <= start || !from && !to) throw new Error('请填写至少一个时间端点，结束须晚于开始。');
      onChange({ from: from || null, to: to || null });
    } catch (error) { setError(error instanceof Error ? error : new Error('时间范围无效。')); }
  }}>
    <p className="text-sm text-muted-foreground">日期选择使用本机时区 {local}；开始包含、结束不含。未改动的端点保留原纳秒精度。</p>
    <Field label="开始日期时间" type="datetime-local" step="1" value={safeLocal(from)} onChange={value => change(value, setFrom)} />
    <Field label="结束日期时间" type="datetime-local" step="1" value={safeLocal(to)} onChange={value => change(value, setTo)} />
    <details className="rounded-md border p-3"><summary className="cursor-pointer text-sm font-medium">精确时间与不限端点</summary><div className="mt-4 space-y-4"><Field label="窗口开始（含）" value={from} onChange={setFrom} hint="RFC 3339，支持 9 位小数；留空为不限开始。" /><Field label="窗口结束（不含）" value={to} onChange={setTo} hint="例如 2026-11-12T20:00:00+08:00；留空为不限结束。" /></div></details>
    <MonitoringError error={error} />
    <div className="flex flex-wrap gap-2"><Button type="submit">应用时间范围</Button><Button type="button" variant="outline" disabled={!earliest || !latest} onClick={() => earliest && latest && onChange({ from: earliest, to: parseNS(latest) === MAX_NS ? null : formatNS(parseNS(latest) + 1n) })}>全部历史范围</Button></div>
  </form>;
}
