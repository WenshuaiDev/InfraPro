import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { usePagedQuery } from '@infrapro/ui/hooks/use-paged-query';
import type { DeviceSourceTarget } from '@infrapro/api-client';
import { ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { useDevices } from '../devices/context';
import { Equipment } from './shared';

export function BindingHistory({ target }: { target: DeviceSourceTarget }) {
  const { client, scope, prefix, canSourceHistory } = useDevices();
  const query = usePagedQuery({ initialPageSize: 10, queryKey: [...prefix, 'source-history', target], enabled: canSourceHistory, queryFn: ({ cursor, signal, limit }) => client.deviceSourceHistory(scope, target, { cursor, limit }, signal) });
  if (!canSourceHistory) return <p className="text-sm text-muted-foreground">当前权限不包含绑定历史查看。</p>;
  return <section className="space-y-4 border-t pt-4" aria-label="绑定历史"><h3 className="font-semibold">绑定历史</h3><p className="text-xs text-muted-foreground">按生效时间排列；更换和解除后，原关系仍会保留。</p>{query.isPending ? <LoadingState /> : query.error ? <ErrorState message={query.error.message} onRetry={() => void query.refetch()} /> : !query.data?.items.length ? <p className="text-sm text-muted-foreground">暂无绑定记录。</p> : <ol className="space-y-3">{query.data.items.map((binding, i) => <li key={binding.configured && binding.visibility === 'full' ? binding.id : i} className="space-y-2 rounded-lg border p-4">{binding.configured && binding.visibility === 'full' ? <>{binding.output && <Equipment output={binding.output} />}<p className="text-xs text-muted-foreground">{binding.started_at ? new Date(binding.started_at).toLocaleString() : '时间不可用'} → {binding.ended_at ? new Date(binding.ended_at).toLocaleString() : '当前生效'}</p>{binding.reason && <p className="whitespace-pre-wrap break-words text-sm">绑定原因：{binding.reason}</p>}</> : <p className="text-sm text-muted-foreground">此段绑定的设备及时间详情受权限限制。</p>}</li>)}</ol>}{query.data && <ListPagination label="记录" {...query.pagination} />}</section>;
}
