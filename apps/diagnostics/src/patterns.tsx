import { label, timestamp } from './format';
import { Component, useEffect, useRef, useState, type ReactNode } from 'react';
import { AlertCircle, CheckCircle2, CircleHelp, Clock3, PauseCircle } from 'lucide-react';
import { Badge } from '@infrapro/ui/components/badge';
import { Card, CardContent } from '@infrapro/ui/components/card';
import { DiagnosticsError, type Coverage, type Meta } from '@infrapro/diagnostics-client';
export function Status({ value, stale = false }: { value: string; stale?: boolean }) {
  const positive = !stale && ['healthy', 'ready', 'running', 'fresh', 'ok', 'complete'].includes(value);
  const negative = !stale && ['unavailable', 'not_ready', 'stopped', 'missing', 'blocked', 'stalled', 'failed'].includes(value);
  const warning = !stale && ['degraded', 'partial', 'unconfigured', 'clock_uncertain'].includes(value);
  const Icon = positive ? CheckCircle2 : negative || warning ? AlertCircle : value === 'disabled' ? PauseCircle : ['stale', 'historical'].includes(value) ? Clock3 : CircleHelp;
  return <Badge variant={negative ? 'destructive' : positive ? 'default' : 'secondary'} className={`gap-1 whitespace-nowrap ${warning ? 'border-amber-600/40 bg-amber-50 text-amber-900 dark:bg-amber-950 dark:text-amber-100' : ''}`}><Icon className="size-3" aria-hidden="true" />{stale ? `历史：${label(value)}` : label(value)}</Badge>;
}
export function CoverageText({ value, stale = false }: { value: Coverage; stale?: boolean }) { return <span className="inline-flex flex-wrap items-center gap-2"><Status value={value.state} stale={stale} /><span>{value.fresh}/{value.expected} 新鲜 · {value.unavailable} 项缺口</span></span>; }
export function Panel({ title, children }: { title: string; children: ReactNode }) { return <Card><CardContent className="space-y-4 pt-5"><h2 className="text-base font-semibold">{title}</h2>{children}</CardContent></Card>; }
export function DataTable({ children }: { children: ReactNode }) { return <div className="overflow-x-auto rounded-lg border"><table className="diagnostic-table">{children}</table></div>; }
export function Feedback({ error, meta, pending }: { error: Error | null; meta?: Meta; pending?: boolean }) {
  if (error instanceof DiagnosticsError && error.code === 'target_removed') return <div role="status" className="rounded-lg border p-4 text-sm"><strong>配置已更新，此目标已移除。</strong><p>请从组件或节点目录选择当前目标。</p></div>;
  if (error) return <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-4 text-sm"><strong>无法连接诊断服务或读取此视图。</strong><p>{error.message}</p>{meta && <p>以下为 {timestamp(meta.server_time)} 的历史数据，当前状态尚未确认。</p>}</div>;
  if (pending) return <p className="py-8 text-sm text-muted-foreground">正在读取诊断证据…</p>;
  return null;
}
export class RegionBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  render() { return this.state.failed ? <div className="rounded border p-4 text-sm">此区域暂时无法显示，其他证据仍可查看。</div> : this.props.children; }
}
export function LiveRegion({ message }: { message: string }) {
  const [announced, setAnnounced] = useState(''); const last = useRef(0);
  useEffect(() => { const delay = Math.max(0, 15000 - (Date.now() - last.current)); const timer = window.setTimeout(() => { setAnnounced(message); last.current = Date.now(); }, delay); return () => window.clearTimeout(timer); }, [message]);
  return <div aria-live="polite" aria-atomic="true" className="sr-only">{announced}</div>;
}
