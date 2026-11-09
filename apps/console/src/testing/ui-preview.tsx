// Separate Vite HTML entry used only by component tests; excluded from production inputs.
import { useState } from 'react';
import { ChartColumn, House, Landmark, Settings } from 'lucide-react';
import { AdminLayout, AuthCard, AuthLayout, PageContent, PageHeader, SettingsLayout, type LinkRenderer } from '@infrapro/layouts';
import { useLayoutStore } from '@infrapro/layouts/state';
import { Button } from '@infrapro/ui/components/button';
import { Card, CardContent } from '@infrapro/ui/components/card';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { PasswordInput } from '@infrapro/ui/patterns/password-input';
import { ConfirmDialog } from '@infrapro/ui/patterns/confirm-dialog';
import { SeriesChart } from '@infrapro/ui/charts';
import '../styles/global.css';

const brand = { name: 'InfraPro', description: '工程工作空间', logo: <Landmark /> };
const data = [{ month: '一月', measured: 12, reference: 10 }, { month: '二月', measured: 18, reference: 14 }, { month: '三月', measured: 15, reference: 12 }, { month: '四月', measured: 24, reference: 20 }];
const series = [{ key: 'measured', label: '测试系列一' }, { key: 'reference', label: '测试系列二' }];

export function Preview() {
  const [path, setPath] = useState('/');
  const [state, setState] = useState('data');
  const [confirm, setConfirm] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const reset = useLayoutStore((value) => value.reset);
  const navigation = [{ title: '首页', href: '/', icon: House }, { title: '图表组件', href: '/charts', icon: ChartColumn }, { title: '设置', href: '/settings', icon: Settings }].map((item) => ({ ...item, active: path === item.href }));
  const renderLink: LinkRenderer = ({ href, onClick, ...props }) => <a href={href} {...props} onClick={(event) => { if (event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey) { event.preventDefault(); onClick?.(event); setPath(href); } }} />;
  if (new URLSearchParams(location.search).get('view') === 'auth') return <AuthLayout brand={brand}><AuthCard title="欢迎回来" description="使用组织邀请创建的账号登录。" footer="没有账号？请联系同事获取邀请链接。"><form className="grid gap-3" onSubmit={(event) => event.preventDefault()}><div className="grid gap-2"><Label htmlFor="username">用户名</Label><Input id="username" autoComplete="username" /></div><div className="grid gap-2"><div className="flex justify-between"><Label htmlFor="password">密码</Label><a href="?view=auth" className="text-sm font-medium text-muted-foreground">忘记密码</a></div><PasswordInput id="password" autoComplete="current-password" /></div><Button className="mt-2">登录</Button></form></AuthCard></AuthLayout>;
  return <AdminLayout brand={brand} groups={[{ title: '工作空间', items: navigation }]} user={{ displayName: '组件自验', username: 'fixture' }} userNavigation={navigation.slice(2)} renderLink={renderLink} onNavigate={setPath} onLogout={() => setConfirmed(true)}>
    <PageContent><PageHeader title={navigation.find((item) => item.href === path)?.title ?? '组件自验'} description="以下为隔离的组件测试数据，不进入正式产品页面。" actions={<Button variant="outline" onClick={reset}>重置布局偏好</Button>} />
      {path === '/settings' ? <SettingsLayout navigation={navigation.slice(2)} renderLink={renderLink}><h2 className="font-semibold">设置内容</h2></SettingsLayout> : <>
        <div className="mb-6 flex flex-wrap gap-2">{[['data', '有数据'], ['empty', '空数据'], ['loading', '加载中'], ['error', '错误']].map(([value, label]) => <Button key={value} variant={state === value ? 'default' : 'outline'} onClick={() => setState(value!)}>{label}</Button>)}</div>
        <div className="grid min-w-0 gap-6 xl:grid-cols-2">{(['line', 'area', 'bar', 'pie', 'donut'] as const).map((kind) => <Card key={kind}><CardContent><SeriesChart kind={kind} title={`${kind} 图表`} description="固定数据夹具" data={state === 'empty' ? [] : data} categoryKey="month" series={kind === 'pie' || kind === 'donut' ? series.slice(0, 1) : series} loading={state === 'loading'} error={state === 'error' ? '测试加载失败' : undefined} onRetry={() => setState('data')} /></CardContent></Card>)}</div>
      </>}
      <div className="mt-6"><ConfirmDialog trigger={<Button variant="outline">测试确认弹窗</Button>} open={confirm} onOpenChange={setConfirm} title="确认操作" description="仅验证组件交互，不执行业务操作。" onConfirm={() => { setConfirmed(true); setConfirm(false); }} /></div>{confirmed && <p role="status">操作已确认</p>}
    </PageContent>
  </AdminLayout>;
}
