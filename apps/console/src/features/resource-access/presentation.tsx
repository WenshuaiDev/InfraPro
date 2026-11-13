/* eslint-disable react-refresh/only-export-components */
import type { ReactNode } from 'react';
import { Building2, Info, ListChecks, ShieldCheck } from 'lucide-react';
import { Badge } from '@infrapro/ui/components/badge';
import { kinds, describe, type Capability, type Choice, type Summary } from './model';
import type { ResourceKind } from '@infrapro/api-client';

export function Hint({ children }: { children: ReactNode }) {
  return (
    <div className="flex items-start gap-2 rounded-lg border bg-muted/25 p-3 text-sm leading-relaxed text-muted-foreground">
      <Info className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <div className="min-w-0">{children}</div>
    </div>
  );
}
export function AccessSteps() {
  return (
    <div className="flex flex-wrap gap-x-8 gap-y-3 rounded-xl border bg-muted/25 px-5 py-4 text-sm">
      {[
        { Icon: Building2, label: '1. 选设施' },
        { Icon: ListChecks, label: '2. 选资源、设权限' },
        { Icon: ShieldCheck, label: '3. 核对并授权' },
      ].map(({ Icon, label }) => (
        <span key={label} className="flex items-center gap-2">
          <Icon className="size-4 text-muted-foreground" aria-hidden="true" />
          {label}
        </span>
      ))}
    </div>
  );
}
export function ResourceCounts({ value }: { value: Summary }) {
  return (
    <span>
      {value.components || value.points || value.channels
        ? `${value.components} 个构件 · ${value.points} 个测点 · ${value.channels} 个通道`
        : '仅设施基本档案'}
    </span>
  );
}
export type Difference = {
  id: string;
  name: string;
  code?: string;
  kind?: ResourceKind;
  before: string;
  after: string;
  action: 'add' | 'update' | 'end';
};
export function choiceDifferences(choices: Choice[]): Difference[] {
  return choices.map((v) => ({
    id: v.id,
    name: v.name,
    code: v.code,
    kind: v.kind,
    before: describe(v.grant?.capabilities),
    after: v.end ? '收回当前项目的授权' : describe(v.capabilities),
    action: v.end ? 'end' : v.grant ? 'update' : 'add',
  }));
}
export function ChangeList({ items }: { items: Difference[] }) {
  return (
    <ul className="divide-y rounded-lg border">
      {items.map((v) => (
        <li key={v.id} className="space-y-2 p-3 text-sm">
          <div className="flex items-start justify-between gap-3">
            <p className="min-w-0 break-words font-medium">
              {v.name}
              <span className="ml-2 text-xs font-normal text-muted-foreground">
                {v.kind && kinds[v.kind]}
                {v.code && ` · ${v.code}`}
              </span>
            </p>
            <Badge variant="outline" className="shrink-0 font-normal">
              {{ add: '新增', update: '调整', end: '收回' }[v.action]}
            </Badge>
          </div>
          <div className="grid gap-1 text-xs leading-relaxed">
            <p className="text-muted-foreground">变更前：{v.before}</p>
            <p>变更后：{v.after}</p>
          </div>
        </li>
      ))}
    </ul>
  );
}
export function capsDescription(value: readonly Capability[] | undefined) {
  return describe(value);
}
