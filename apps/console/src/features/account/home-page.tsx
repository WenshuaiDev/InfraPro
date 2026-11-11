import { Link } from '@tanstack/react-router';
import { ArrowUpRight, MailPlus, ShieldCheck, UserRound, Building2 } from 'lucide-react';
import { useIdentity } from '@infrapro/api-client/react';
import { PageHeader } from '@infrapro/layouts';
import { Card, CardContent, CardDescription, CardHeader } from '@infrapro/ui/components/card';
import { Badge } from '@infrapro/ui/components/badge';
import { allowedPath } from '../../config/navigation';
import { date } from '../auth/forms';

export function HomePage() {
  const { account, can, permissions, help } = useIdentity();
  if (!account) return null;
  return <><PageHeader title="账号首页" description="管理你的账号，连接工程工作空间。" />
    {permissions.length === 0 && <section className="mb-6 space-y-2 rounded-lg border p-5"><h2 className="font-semibold">尚未分配可用权限</h2><p>当前没有本人及平台权限。你仍可查看我的租户，或联系维护人员配置角色。</p><p className="text-sm text-muted-foreground">{help?.support_contact}</p></section>}
    <div className="grid gap-6 lg:grid-cols-3"><Card className="lg:col-span-2"><CardHeader className="flex flex-row items-start justify-between"><div className="space-y-1"><h2 className="font-semibold">当前账号</h2><CardDescription>你的基本资料与身份信息。</CardDescription></div><Badge variant="secondary">已登录</Badge></CardHeader><CardContent><dl className="grid gap-x-8 gap-y-6 sm:grid-cols-2"><div><dt className="text-sm text-muted-foreground">显示名称</dt><dd className="mt-1 font-medium">{account.display_name}</dd></div><div><dt className="text-sm text-muted-foreground">用户名</dt><dd className="mt-1 font-medium">{account.username}</dd></div><div className="sm:col-span-2"><dt className="text-sm text-muted-foreground">创建时间</dt><dd className="mt-1 text-sm"><time dateTime={account.created_at}>{date(account.created_at)}</time></dd></div></dl></CardContent></Card>
      <Card><CardHeader><h2 className="font-semibold">账号使用提示</h2><CardDescription>维护账号，安全地邀请同事加入。</CardDescription></CardHeader><CardContent className="space-y-3 text-sm leading-relaxed text-muted-foreground"><p>在个人资料中更新显示名称，在账号安全中检查登录设备。</p><p>邀请链接仅能使用一次，请交给需要加入的同事。</p></CardContent></Card>
    </div><div className="mt-6 grid gap-4 md:grid-cols-3">{[
      { href: '/tenants' as const, title: '我的租户', description: '进入自己的租户，维护资料与操作记录。', icon: Building2 },
      { href: '/profile' as const, title: '个人资料', description: '查看账号信息，更新显示名称。', icon: UserRound },
      { href: '/security' as const, title: '账号安全', description: '修改密码，管理会话和安全记录。', icon: ShieldCheck },
      { href: '/invitations' as const, title: '账号开户邀请', description: '邀请同事加入，查看链接使用状态。', icon: MailPlus },
    ].filter((item) => allowedPath(item.href, can)).map((item) => <Link key={item.href} to={item.href} className="group rounded-xl border bg-card p-5 shadow-sm transition-colors hover:bg-accent/40 focus-visible:outline-2 focus-visible:outline-ring"><div className="mb-4 flex items-center justify-between"><item.icon className="size-5 text-muted-foreground" /><ArrowUpRight className="size-4 text-muted-foreground" /></div><h2 className="font-semibold">{item.title}</h2><p className="mt-1 text-sm text-muted-foreground">{item.description}</p></Link>)}</div>
  </>;
}
