import { CheckCircle2, CircleAlert, CircleDashed } from 'lucide-react';
import { Badge } from '@infrapro/ui/components/badge';

export function TenantListStatus({ status }: { status: string }) {
  const Icon = status === 'active' ? CheckCircle2 : CircleDashed;
  return <span className="inline-flex items-center gap-2 whitespace-nowrap text-sm"><Icon className="size-4 text-muted-foreground" aria-hidden="true" />{status === 'active' ? '已启用' : '已停用'}</span>;
}

export function AdministratorAvailability({ available }: { available: boolean }) {
  return available ? <Badge variant="outline" className="font-normal">正常</Badge> : <span className="inline-flex items-center gap-1.5 text-sm text-destructive"><CircleAlert className="size-4 shrink-0" aria-hidden="true" />管理员不可用</span>;
}

export function TenantTime({ value }: { value: string }) {
  const date = new Date(value);
  return <time dateTime={value} title={`${date.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false })}（Asia/Shanghai）`} className="block space-y-1 text-xs tabular-nums"><span className="block">{date.toLocaleDateString('zh-CN', { timeZone: 'Asia/Shanghai' })}</span><span className="block text-muted-foreground">{date.toLocaleTimeString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false })}</span></time>;
}
