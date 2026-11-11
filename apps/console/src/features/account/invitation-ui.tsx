import { Ban, CheckCircle2, ChevronRight, Clock3, FileClock, MailPlus, Plus, RotateCcw, Search, ShieldCheck, Users } from 'lucide-react';
import type { Invitation } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { cn } from '@infrapro/ui/lib/utils';
import { invitationStatuses, invitationDate } from './invitation-utils';

const statusIcons = { pending: Clock3, used: CheckCircle2, expired: FileClock, revoked: Ban };
export function InvitationStatus({ status }: { status: Invitation['status'] }) {
  const Icon = statusIcons[status];
  return <Badge variant="outline" className={cn('gap-1.5 whitespace-nowrap rounded-md py-1 font-medium', status === 'pending' && 'border-blue-200 bg-blue-50 text-blue-700 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-300', status === 'used' && 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-300', (status === 'revoked' || status === 'expired') && 'text-muted-foreground')}><Icon className="size-3" aria-hidden="true" />{invitationStatuses[status]}</Badge>;
}
export function InvitationExpiry({ item, checkedAt }: { item: Invitation; checkedAt: number }) {
  const remaining = new Date(item.expires_at).getTime() - checkedAt;
  return <div className="space-y-1"><time dateTime={item.expires_at} className="text-muted-foreground tabular-nums">{invitationDate(item.expires_at)}</time>{item.status === 'pending' && remaining > 0 && remaining <= 3_600_000 && <span className="block w-fit rounded bg-amber-50 px-1.5 py-0.5 text-xs text-amber-800 dark:bg-amber-950 dark:text-amber-300">即将过期</span>}</div>;
}
export function InvitationRules({ compact = false }: { compact?: boolean }) {
  if (compact) return <p className="flex items-start gap-2 text-sm leading-6 text-muted-foreground"><ShieldCheck className="mt-1 size-4 shrink-0" aria-hidden="true" /><span>每条链接仅可创建 1 个基础成员账号，不继承你的角色。</span></p>;
  return <div className="space-y-3 rounded-lg bg-muted/50 p-4 text-sm leading-6"><p className="flex items-start gap-2.5"><Users className="mt-1 size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><span>每条链接创建 <strong className="font-medium">1 个基础成员账号</strong>，不继承你的角色。</span></p><p className="flex items-start gap-2.5"><ShieldCheck className="mt-1 size-4 shrink-0 text-muted-foreground" aria-hidden="true" /><span>持有链接的人均可认领。请私下交付，疑似泄露时及时撤销。</span></p></div>;
}
export function InvitationEmpty({ filtered = false, create, reset }: { filtered?: boolean; create?: () => void; reset?: () => void }) {
  const Icon = filtered ? Search : MailPlus;
  return <section className="rounded-xl border px-6 py-14 text-center" aria-labelledby="invitation-empty-title"><div className="mx-auto mb-5 flex size-14 items-center justify-center rounded-2xl border bg-muted/40"><Icon className="size-6 text-muted-foreground" aria-hidden="true" /></div><h2 id="invitation-empty-title" className="text-base font-semibold">{filtered ? '没有符合条件的邀请' : create ? '从第一条邀请开始' : '还没有发出的邀请'}</h2><p className="mx-auto mt-2 max-w-sm text-sm leading-6 text-muted-foreground">{filtered ? '试试其他关键词，或清除筛选查看全部记录。' : create ? '生成一条专属链接，发给需要加入的同事。对方完成开户后，你可以在这里查看状态。' : '这里会显示你发出的邀请记录和使用状态。'}</p>{filtered ? <Button className="mt-6" variant="outline" onClick={reset}><RotateCcw />清除筛选</Button> : create && <><Button className="mt-6" onClick={create}><Plus />创建第一条邀请</Button><div className="mt-8 flex flex-wrap items-center justify-center gap-3 text-xs text-muted-foreground"><span>1. 生成链接</span><ChevronRight className="size-3" aria-hidden="true" /><span>2. 私下交付</span><ChevronRight className="size-3" aria-hidden="true" /><span>3. 对方开户</span></div></>}</section>;
}
