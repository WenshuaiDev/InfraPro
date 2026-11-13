import { Link } from '@tanstack/react-router';
import { TableRow, TableCell } from '@infrapro/ui/components/table';
import { Badge } from '@infrapro/ui/components/badge';
import type { SelectedChannel } from './channel-picker';
import { useMonitoring } from './context';
import { qualityNames } from './format';
import { calculationStatusNames, reasonNames, type DetailRow } from './detail-format';


export function MonitoringDetailRow({ row, channels }: { row: DetailRow; channels: SelectedChannel[] }) {
  const { paths, canImportRead } = useMonitoring();
  const channel = channels.find(item => item.id === row.channel_id);
  const name = channel ? `${channel.owner_name} · ${channel.name}` : row.channel_id;
  if (row.source_kind === 'derived') return <TableRow>
    <TableCell className="font-mono text-xs">{row.output_at}{row.interval_start && <p className="mt-1 text-muted-foreground">窗口起点 {row.interval_start}</p>}</TableCell>
    <TableCell>{name}<p className="mt-1 text-xs text-muted-foreground">派生监测</p></TableCell>
    <TableCell className="font-mono">{row.value ?? '—'} {channel?.unit}<p className="mt-1 text-xs text-muted-foreground">{row.scale} 位小数{row.rounded ? ' · 已舍入' : ''}</p></TableCell>
    <TableCell className="max-w-64 whitespace-normal"><Badge variant="outline">{qualityNames[row.data_quality]}</Badge><p className="mt-1 text-sm">{calculationStatusNames[row.calculation_status]}</p>{row.reason_flags.length > 0 && <p className="mt-1 text-xs text-muted-foreground">{row.reason_flags.map(reason => reasonNames[reason] ?? reason).join('；')}</p>}<p className="mt-1 text-xs">纳入 {row.sample_count} · 排除 {row.excluded_count}</p></TableCell>
    <TableCell className="max-w-80 whitespace-normal"><Badge variant="secondary">{row.freshness === 'current' ? '当前有效' : '历史结果'}</Badge><details className="mt-2 text-xs"><summary className="min-h-11 cursor-pointer py-3">计算依据</summary><dl className="space-y-1 break-all"><dt className="text-muted-foreground">配置版本</dt><dd>{row.config_id}</dd><dt className="text-muted-foreground">发布代次</dt><dd>{row.generation_id}</dd><dt className="text-muted-foreground">计算运行</dt><dd>{row.run_id}</dd><dt className="text-muted-foreground">实际输入时段</dt><dd>{row.input_first ?? '无样本'}{row.input_last && <> 至 {row.input_last}</>}</dd></dl></details></TableCell>
  </TableRow>;
  return <TableRow>
    <TableCell className="font-mono text-xs">{row.observed_at}{row.interval_start && <p className="mt-1 text-muted-foreground">区间起点 {row.interval_start}</p>}</TableCell>
    <TableCell>{name}</TableCell><TableCell className="font-mono">{row.value} {channel?.unit}</TableCell>
    <TableCell><Badge variant="outline">{qualityNames[row.quality]}</Badge>{row.quality_reason && <p className="mt-1 max-w-64 whitespace-normal text-xs">{row.quality_reason}</p>}</TableCell>
    <TableCell>{canImportRead ? <Link to={paths.batch(row.batch_id)} className="underline">查看来源批次</Link> : '来源信息受限'}{row.revoked && <p className="mt-1 text-xs">已撤销，历史观测</p>}<details className="mt-1 text-xs"><summary className="min-h-11 cursor-pointer py-3">观测标识</summary><p>{row.observation_id}</p><p>采用 {row.adoption_id}</p></details></TableCell>
  </TableRow>;
}
