import { Link } from '@tanstack/react-router';
import { Activity, BookOpen } from 'lucide-react';
import { MonitoringPageHeader } from '../monitoring/page-header';
import { useDerived } from './common';
import { DerivedChannelsPage } from './channels-page';
import { DerivedDefinitionsPage } from './definitions-page';
import type { DerivedSearch } from './search';
import { derivedWorkspaceTab } from './workspace-model';

export function DerivedWorkspacePage(search: DerivedSearch) {
  const m = useDerived();
  const tab = derivedWorkspaceTab(search.tab, m.allows('channels.read'));
  const tabs = [
    { key: 'channels' as const, title: '监测通道', icon: Activity, allowed: m.allows('channels.read') },
    { key: 'definitions' as const, title: '指标定义', icon: BookOpen, allowed: m.allows('metrics.read') },
  ].filter(item => item.allowed);
  return <div className="min-w-0 space-y-5">
    <MonitoringPageHeader workspaceTasks title="派生监测" description="定义可复用的计算规则，绑定工程对象，持续跟踪监测结果。" />
    <nav aria-label="派生监测视图" className="flex gap-1 border-b">{tabs.map(item => <Link key={item.key} to={m.derivedPath} search={{ tab: item.key }} aria-current={tab === item.key ? 'page' : undefined} className={`flex min-h-12 items-center gap-2 border-b-2 px-4 text-sm font-medium ${tab === item.key ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:bg-muted/40 hover:text-foreground'}`}><item.icon className="size-4" aria-hidden />{item.title}</Link>)}</nav>
    {tabs.length === 0 ? <p role="status">当前身份没有派生监测读取权限。</p> : tab === 'definitions' ? <DerivedDefinitionsPage {...search} /> : <DerivedChannelsPage {...search} />}
  </div>;
}
