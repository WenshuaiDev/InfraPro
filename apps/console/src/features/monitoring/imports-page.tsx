import { useEffect, useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import { useQueryClient } from '@tanstack/react-query';
import { Upload } from 'lucide-react';
import { PageHeader } from '@infrapro/layouts';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { ErrorState } from '@infrapro/ui/patterns/feedback';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useMonitoring } from './context';
import { CursorButtons, MonitoringTabs } from './common';
import { count, stateNames } from './format';
import { UploadForm } from './upload-form';

export function MonitoringImportsPage() {
  const m = useMonitoring(), cache = useQueryClient(), navigate = useNavigate(); const [upload, setUpload] = useState(false);
  const list = useCursorList({ queryKey: [...m.prefix, 'imports'], load: (cursor, signal) => m.client.monitoringImports(m.scope, cursor, signal), enabled: m.available && m.canImportRead });
  const { refetch } = list;
  useEffect(() => { const timer = setInterval(() => { if (document.visibilityState === 'visible') void refetch(); }, 5000); return () => clearInterval(timer); }, [refetch]);
  if (!m.canImportRead) return <ErrorState title="没有导入批次查看权限" message="请联系当前工作空间管理员配置权限。" />;
  return <section className="min-w-0 space-y-5"><PageHeader title="导入与批次" description="文件先封存，再明确映射和全量校验；合格批次统一生效。" actions={m.canManage && <Button id="new-monitoring-import" disabled={!m.available} onClick={() => setUpload(true)}><Upload aria-hidden />导入历史文件</Button>} /><MonitoringTabs current="imports" /><DirectoryTable label="监测导入批次" items={list.items} rowId={v => v.id} loading={list.isPending} error={list.error?.message} retry={() => void list.refetch()} empty="当前范围暂无导入批次。CSV 和 XLSX 可用于导入已存在业务通道的历史数据。" columns={[{ id: 'file', label: '文件 / 批次', cell: b => <div className="min-w-0"><Link to={m.paths.batch(b.id)} className="break-words font-medium underline underline-offset-4">{b.name}</Link><p className="mt-1 text-xs text-muted-foreground">{new Date(b.created_at).toLocaleString()}</p>{b.scope_clipped && <p className="text-xs">仅当前获准部分</p>}</div> }, { id: 'state', label: '处理状态', cell: b => <Badge variant="outline">{stateNames[b.state] ?? b.state}</Badge> }, { id: 'count', label: '数量', cell: b => b.scope_clipped ? <span>可见采用 {count(b.visible_adopted_count)}</span> : b.counts ? <div className="text-sm"><p>候选 {count(b.counts.candidates)} · 拟采用 {count(b.counts.proposed)}</p><p className="text-xs text-muted-foreground">错误 {count(b.counts.errors)} · 可疑 {count(b.counts.suspect)} · 忽略 {count(b.counts.ignored)}</p></div> : <span className="text-sm">文件 {count(b.size_bytes)} 字节</span> }]} pagination={<CursorButtons previous={list.page ? () => list.go(list.page - 1) : undefined} next={list.hasNext ? () => void list.next() : undefined} busy={list.isFetching} caption={`批次第 ${list.page + 1} 页`} />} /><ResponsivePanel open={upload} onOpenChange={setUpload} title="导入历史文件" description="先上传并封存文件，再核对列、单位、时间和目标通道。" fallbackFocusSelector="#new-monitoring-import">{upload && <UploadForm done={batch => { setUpload(false); void cache.invalidateQueries({ queryKey: [...m.prefix, 'imports'] }); void navigate({ to: m.paths.batch(batch.id) }); }} />}</ResponsivePanel></section>;
}
