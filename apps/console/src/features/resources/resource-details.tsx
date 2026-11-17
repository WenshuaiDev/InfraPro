import { BoreholeFacts } from './borehole-fields';
import { useRef } from 'react';
import type { EngineeringField, ResourceDetail } from '@infrapro/api-client';
import { MoreHorizontal } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { PositionView } from '../location-fields/position-editor';
const labels = { facility: '设施', component: '构件', monitoring_section: '监测断面', borehole: '孔位', point: '测点', channel: '通道' };
const modes = { edit: '编辑', retire: '退役', restore: '恢复', delete: '删除' };
function fieldValue(field: EngineeringField, value: unknown) {
  if (value === undefined || value === null) return '未填写';
  const option = (key: unknown) => field.options?.find((item) => item.key === key)?.name ?? String(key);
  if (field.shape === 'enum') return option(value);
  if (field.shape === 'multi_enum' && Array.isArray(value)) return value.map(option).join('、') || '未填写';
  if (typeof value === 'boolean') return value ? '是' : '否';
  if (field.shape === 'datetime' && typeof value === 'string') return value.replace('T', ' ').replace(/(Z|[+-]\d{2}:\d{2})$/, ' $1');
  return Array.isArray(value) ? value.join('、') : String(value);
}

export function ObjectActions({
  resource,
  disabled,
  choose,
}: {
  resource: ResourceDetail;
  disabled: boolean;
  choose: (mode: 'edit' | 'retire' | 'restore' | 'delete', trigger: HTMLElement | null) => void;
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  const pendingMode = useRef<'edit' | 'retire' | 'restore' | 'delete' | null>(null);
  if (!['edit', 'retire', 'restore', 'delete'].some((k) => resource.actions[k as 'edit'])) return null;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          ref={trigger}
          id={`resource-actions-${resource.id}`}
          variant="outline"
          size="icon"
          aria-label={`${resource.name} 操作`}
          disabled={disabled}
        >
          <MoreHorizontal aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        onCloseAutoFocus={(event) => {
          const mode = pendingMode.current;
          pendingMode.current = null;
          if (!mode) return;
          // Finish menu teardown before the panel takes focus; exit pointer events can still refocus the menu.
          event.preventDefault();
          choose(mode, trigger.current);
        }}
      >
        {(['edit', 'retire', 'restore', 'delete'] as const)
          .filter((k) => resource.actions[k])
          .map((mode) => (
            <DropdownMenuItem
              key={mode}
              onSelect={() => {
                pendingMode.current = mode;
              }}
            >
              {modes[mode]}
              {labels[resource.kind]}
            </DropdownMenuItem>
          ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
export function ResourceInfo({ resource: r, showIdentity = true }: { resource: ResourceDetail; showIdentity?: boolean }) {
  const type = r.definitions.find((v) => v.kind === 'type-definitions');
  const fields =
    r.kind === 'facility'
      ? type?.content.fields
      : r.kind==='monitoring_section'||r.kind==='borehole'?type?.content.structure?.[r.kind]?.fields:type?.content.components?.find((c) => c.key === r.category_key)?.fields;
  const metric = r.definitions.find((v) => v.id === r.metric_version_id);
  const unit = r.definitions.find((v) => v.id === r.unit_version_id);
  return (
    <div className="min-w-0 space-y-4 text-sm">
      {showIdentity && <><p className="break-words text-muted-foreground">
        归属路径：{[...r.ancestors.map((a) => a.name), r.name].join(' / ')}
      </p>
      <p className="break-all">
        编号：{r.code} · {r.status === 'normal' ? '正常' : '已退役'}
      </p></>}
      {r.kind==='borehole'&&r.borehole&&<BoreholeFacts value={r.borehole}/>}
      {r.actions.reason && <p className="text-muted-foreground">{r.actions.reason}</p>}
      {!!fields?.length && (
        <dl className="grid gap-3 sm:grid-cols-2">
          {fields.map((f) => (
            <div key={f.key} className="min-w-0">
              <dt className="text-muted-foreground">{f.name}</dt>
              <dd className="break-words">
                {fieldValue(f, r.values?.[f.key])}
                {f.unit_version && ` ${r.definitions.find((v) => v.id === f.unit_version)?.content.unit?.symbol ?? ''}`}
              </dd>
            </div>
          ))}
        </dl>
      )}
      {(
        [
          ['业务说明', r.description],
          ['监测目的', r.purpose],
          ['方向说明', r.direction_description],
          ['参考基准', r.reference_description],
          ['资料来源', r.source_reference],
        ] as const
      )
        .filter(([, v]) => v)
        .map(([label, value]) => (
          <div key={label}>
            <p className="text-muted-foreground">{label}</p>
            <p className="whitespace-pre-wrap break-words">{value}</p>
          </div>
        ))}
      {r.kind !== 'channel' ? (
        <details>
          <summary className="cursor-pointer font-medium">本对象位置资料</summary>
          <div className="mt-3">
            <PositionView position={r.position} />
          </div>
        </details>
      ) : (
        <div className={showIdentity ? 'space-y-2' : 'grid gap-4 sm:grid-cols-2'}>
          <p>
            指标：{metric?.content.name} · V{metric?.number} ·{' '}
            {metric?.content.metric?.shape === 'enum'
              ? '枚举'
              : metric?.content.metric?.shape === 'integer'
                ? '整数'
                : '数值'}
          </p>
          <p>固定单位：{unit ? `${unit.content.name}（${unit.content.unit?.symbol}）V${unit.number}` : '无单位'}</p>
          {metric?.content.metric?.shape === 'enum' && (
            <p>选项：{metric.content.metric.options.map((o) => `${o.name}（${o.key}）`).join('、')}</p>
          )}
          <p>时间含义：{r.timestamp_meaning === 'interval_end' ? '统计区间结束时刻' : '观测时刻'}</p>
          {r.aggregation_description && <p className="break-words">统计含义：{r.aggregation_description}</p>}
          <p className="break-words">
            观测方向：{r.observation_direction || '未填写'}；观测基准：{r.reference_basis || '未填写'}
          </p>
          <p>
            计划采样：
            {r.sampling
              ? `${{ periodic: '周期', event: '事件', manual: '人工' }[r.sampling.mode]}${r.sampling.mode === 'periodic' ? ` · ${r.sampling.interval_seconds} 秒` : ''}`
              : '未填写'}
          </p>
          {r.sampling?.description && <p className="break-words">{r.sampling.description}</p>}
        </div>
      )}
      {r.kind !== 'facility' && r.ancestors[0] && (
        <details>
          <summary className="cursor-pointer text-muted-foreground">所属设施的位置参照（不代表本对象坐标）</summary>
          <div className="mt-3">
            <PositionView position={'position' in r.ancestors[0] ? r.ancestors[0].position : undefined} />
          </div>
        </details>
      )}
      <p className="text-xs text-muted-foreground">更新于 {new Date(r.updated_at).toLocaleString('zh-CN', { hour12: false })}</p>
    </div>
  );
}
