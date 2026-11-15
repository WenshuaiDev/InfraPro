import { useRef, useState } from 'react';
import { ChartNoAxesCombined, FolderTree, Plus, Database } from 'lucide-react';
import { MonitoringPageHeader } from './page-header';
import { Button } from '@infrapro/ui/components/button';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { EmptyState, ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { useMonitoring } from './context';
import { MonitoringError } from './common';
import { ChannelPicker, type SelectedChannel } from './channel-picker';
import { MonitoringNavigator, useCurrentChannel } from './workspace-navigation';
import { MonitoringReader } from './workspace-reader';
import { WorkspaceOverview } from './workspace-overview';

export function MonitoringDataPage({ initialPoint, initialChannel, onSelect }: { initialPoint?: string; initialChannel?: string; onSelect: (point: string | undefined, channel: string) => void }) {
  const m = useMonitoring();
  const [directory, setDirectory] = useState(false), [comparison, setComparison] = useState(false), [compared, setCompared] = useState<SelectedChannel[]>([]);
  const [comparePicker, setComparePicker] = useState(true);
  const [workspaceOverview, setWorkspaceOverview] = useState(false);
  const pickerToggle = useRef<HTMLButtonElement>(null);
  const current = useCurrentChannel(initialPoint, initialChannel);
  if (!m.canRead) return <ErrorState title="没有监测数据查看权限" message="请在当前工作空间配置监测数据查看权限。" />;
  const select = (point: string | undefined, channel: string) => { onSelect(point, channel); setDirectory(false); };
  const navigator = <MonitoringNavigator current={current.data} initialPoint={initialPoint} select={select} />;
  return <section className="@container/monitoring min-w-0 space-y-4">
    <MonitoringPageHeader title="监测浏览" description="按工程位置查阅最新观测与历史趋势。" actions={<div className="flex flex-wrap gap-2">
      <Button id="monitoring-directory" variant="outline" className="@[56rem]/monitoring:hidden" onClick={() => setDirectory(true)}><FolderTree aria-hidden />设施构件树</Button>
      <Button id="monitoring-workspace-overview" variant="outline" disabled={!m.available} onClick={() => setWorkspaceOverview(true)}><Database aria-hidden />工作空间统计</Button>
      <Button id="monitoring-compare" variant="outline" disabled={!m.available || !m.canNavigate} onClick={() => { if (!compared.length && current.data) setCompared([current.data]); setComparison(true); }}><ChartNoAxesCombined aria-hidden />多通道比较</Button>
    </div>} />
    <div className="grid min-w-0 items-start gap-4 @[56rem]/monitoring:grid-cols-[18rem_minmax(0,1fr)] @[80rem]/monitoring:grid-cols-[20rem_minmax(0,1fr)]">
      <aside aria-label="监测设施构件树" className="sticky top-20 hidden h-[calc(100dvh-12rem)] min-h-80 max-h-[calc(100dvh-6rem)] min-w-0 flex-col overflow-hidden rounded-lg border @[56rem]/monitoring:flex">{navigator}</aside>
      <div className="min-w-0 space-y-4">
        {!m.canNavigate ? <EmptyState title="无法读取通道目录" description="查阅通道需要工程资源查看权限，请联系管理员配置。" /> : current.error ? <MonitoringError error={current.error} retry={() => void current.refetch()} /> : initialChannel && current.isPending ? <LoadingState label="正在读取当前通道…" /> : current.data ? <MonitoringReader key={current.data.id} channels={[current.data]} /> : <EmptyState title="选择一个通道，开始查阅" description="展开左侧设施、构件和测点，点击监测通道即可查看最新值与趋势。多个通道的比较在“多通道比较”中进行。" />}
      </div>
    </div>
    <ResponsivePanel open={directory} onOpenChange={setDirectory} title="设施构件树" description="展开工程层级，点击通道查看数据。" fallbackFocusSelector="#monitoring-directory">{directory && <div className="flex min-h-[24rem] flex-col rounded-lg border">{navigator}</div>}</ResponsivePanel>
    <ResponsivePanel open={comparison} onOpenChange={setComparison} title="多通道比较" description="选择要比较的通道；兼容通道叠加在同一趋势图中，关闭后保留主页面的查阅位置。" size="workspace" fallbackFocusSelector="#monitoring-compare">
      {comparison && <div className="space-y-4"><Button ref={pickerToggle} className="xl:hidden" variant="outline" aria-expanded={comparePicker} onClick={() => setComparePicker(value => !value)}>{comparePicker ? '收起通道选择' : '选择比较通道'} · {compared.length} 个已选</Button><div className="grid min-w-0 items-start gap-6 xl:grid-cols-[19rem_minmax(0,1fr)]">
        <aside aria-label="选择比较通道" className={`${comparePicker ? 'block' : 'hidden'} min-w-0 space-y-4 rounded-lg border p-4 xl:block`}>
          {current.data && !compared.some(c => c.id === current.data!.id) && <Button variant="outline" className="w-full" onClick={() => setCompared(channels => [...channels, current.data!])} disabled={compared.length >= 50}><Plus aria-hidden />加入主页面当前通道</Button>}
          <ChannelPicker selected={compared} onChange={setCompared} initialPoint={current.data?.point_id ?? initialPoint} /><Button className="w-full xl:hidden" disabled={!compared.length} onClick={() => { setComparePicker(false); pickerToggle.current?.focus(); }}>查看 {compared.length} 个通道的比较</Button>
        </aside>
        <div className="min-w-0">{compared.length ? <MonitoringReader key={compared.map(c => c.id).sort().join(':')} channels={compared} comparing /> : <EmptyState title="添加通道，开始比较" description="从左侧选择测点与通道。相同指标、单位、方向和基准的通道会显示在同一张图里。" />}</div>
      </div></div>}
    </ResponsivePanel>
    <ResponsivePanel open={workspaceOverview} onOpenChange={setWorkspaceOverview} title="工作空间统计" description="当前工作空间内全部获准通道的历史总览，与主页面的通道和时间范围无关。" size="wide" fallbackFocusSelector="#monitoring-workspace-overview">{workspaceOverview && <WorkspaceOverview />}</ResponsivePanel>
  </section>;
}
