import { useCallback, useMemo, useState } from 'react';
import { Outlet, useLocation, useNavigate } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, type MonitoringImportBatch, type MonitoringImportFilter } from '@infrapro/api-client';
import { BookOpen, RefreshCw, Upload } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { MonitoringPageHeader } from './page-header';
import { useMonitoring } from './context';
import { UploadForm } from './upload-form';
import { ImportTable } from './import-table';
import { importFilter, type ImportSearch, type ImportTab } from './import-search';
import { MonitoringError } from './common';

declare module '@tanstack/react-router' { interface HistoryState { importTrail?: (string | null)[]; importTrailFor?: string; importReturnFocus?: string } }
const empty: MonitoringImportBatch[] = [];
export function MonitoringImportsPage({ search }: { search: ImportSearch }) {
  const m = useMonitoring(), cache = useQueryClient(), navigate = useNavigate();
  const history = useLocation({ select: location => location.state });
  const [upload, setUpload] = useState(false), [guide, setGuide] = useState(false);
  const { q, states, formats, sort, limit, cursor } = importFilter(search);
  const filter = useMemo(() => ({ q, states, formats, sort, limit, cursor }), [q, states, formats, sort, limit, cursor]);
  const list = useQuery({ queryKey: [...m.prefix, 'imports', filter], queryFn: ({ signal }) => m.client.monitoringImportList(m.scope, filter, signal), enabled: m.available && m.canImportRead, refetchInterval: 10000, refetchIntervalInBackground: false, refetchOnWindowFocus: 'always', retry: false });
  const trailKey = JSON.stringify([m.scope, { ...filter, cursor: undefined }]);
  const previous = history.importTrailFor === trailKey ? history.importTrail ?? [] : [];
  const change = useCallback((value: Partial<MonitoringImportFilter>) => { void navigate({ to: m.paths.imports, search: { ...filter, ...value, cursor: undefined, tab: undefined }, state: state => ({ ...state, importTrail: [], importTrailFor: undefined }), resetScroll: false }); }, [filter, m.paths.imports, navigate]);
  const base = m.paths.imports;
  const open = useCallback((batch: MonitoringImportBatch, tab?: ImportTab) => { void navigate({ to: `${base}/${encodeURIComponent(batch.id)}`, search: { ...filter, tab }, state: state => ({ ...state, importReturnFocus: `import-${batch.id}` }), resetScroll: false }); }, [base, filter, navigate]);
  const ready = (batch: MonitoringImportBatch) => { setUpload(false); void cache.invalidateQueries({ queryKey: [...m.prefix, 'imports'] }); open(batch, 'mapping'); };
  if (!m.canImportRead) return <ErrorState title="没有导入批次查看权限" message="请联系当前工作空间管理员配置权限。" />;
  return <section className="min-w-0 space-y-6">
    <MonitoringPageHeader title="导入与批次" description="管理历史文件导入，核对数据质量，跟进每个批次的处理结果。" actions={<><Button variant="outline" className="h-9" onClick={() => setGuide(true)}><BookOpen aria-hidden />导入说明</Button>{m.canManage && <Button id="new-monitoring-import" className="h-9" disabled={!m.available} onClick={() => setUpload(true)}><Upload aria-hidden />导入历史文件</Button>}</>} />
    <MonitoringError error={list.error} retry={() => void list.refetch()} />
    {list.error instanceof ApiError && list.error.code === 'monitoring_import_cursor_stale' && <Button variant="outline" onClick={() => change({})}>按当前权限返回第一页</Button>}
    <ImportTable items={!m.available || list.error ? empty : list.data?.items ?? empty} filter={filter} loading={m.available && list.isPending} busy={list.isFetching || !m.available} error={!m.available || !!list.error} next={!!list.data?.next_cursor} previous={!!cursor && previous.length > 0} page={!cursor ? 1 : previous.length ? previous.length + 1 : undefined} change={change} open={open}
      onPrevious={() => { void navigate({ to: base, search: { ...filter, cursor: previous.at(-1) ?? undefined }, state: state => ({ ...state, importTrail: previous.slice(0, -1), importTrailFor: trailKey }), resetScroll: false }); }}
      onNext={() => { if (list.data?.next_cursor) void navigate({ to: base, search: { ...filter, cursor: list.data.next_cursor }, state: state => ({ ...state, importTrail: [...previous, cursor ?? null], importTrailFor: trailKey }), resetScroll: false }); }} />
    <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground"><p>在当前授权范围内搜索与筛选 · 批次状态自动更新</p><Button variant="ghost" size="sm" className="h-7 text-xs" disabled={list.isFetching} onClick={() => void list.refetch()}><RefreshCw aria-hidden className={list.isFetching ? 'motion-safe:animate-spin' : ''} />刷新数据</Button></div>
    <ResponsivePanel open={upload} onOpenChange={setUpload} title="导入历史文件" description="先上传并封存文件，再核对列、单位、时间和目标通道。" fallbackFocusSelector="#new-monitoring-import">{upload && <UploadForm done={ready} />}</ResponsivePanel>
    <ResponsivePanel open={guide} onOpenChange={setGuide} title="历史文件导入说明" description="原文件封存后，通过明确映射和全量校验统一生效。"><ol className="list-decimal space-y-5 pl-5 text-sm leading-7"><li><strong>上传与封存</strong><p className="text-muted-foreground">支持 CSV（最多 32 GiB）和 XLSX（最多 512 MiB）。离开上传页面会暂停传输，可重新选择同一个文件续传。</p></li><li><strong>配置映射</strong><p className="text-muted-foreground">选择单通道、长表或宽表，明确时间列、时区、单位和已存在的业务通道。系统不会自动换算单位。</p></li><li><strong>全量校验与核对</strong><p className="text-muted-foreground">错误会阻止整批发布；可疑值保留原值，默认不进入数学统计。重复观测须明确核对。</p></li><li><strong>统一生效</strong><p className="text-muted-foreground">确认合格批次后，后台完成写入与核验，再统一生效。原文件和处理记录持续保留。</p></li></ol></ResponsivePanel>
    <Outlet />
  </section>;
}
