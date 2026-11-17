import type { DeviceBindingDirectoryItem, DeviceOutput, DeviceScope, DeviceSourceCurrent, DeviceSourceTarget, DeviceTenantSourcePreviewInput, DeviceProjectSourcePreviewInput } from '@infrapro/api-client';

export type BindingRow = DeviceBindingDirectoryItem;
export type BindingAction = 'bind' | 'switch' | 'unbind';
export type BindingCommand = DeviceTenantSourcePreviewInput | DeviceProjectSourcePreviewInput;
export const stateLabels = { all: '全部', unbound: '待绑定', bound: '已绑定', attention: '需处理', restricted: '详情受限', retired: '已退役' };
export const actionLabels = { bind: '绑定设备', switch: '更换设备', unbind: '解除绑定' };
export function bindingLocation(row: BindingRow) {
  return [row.facility_name, ...(row.component_path.length ? row.component_path.map(component => component.name) : ['设施直属（无构件）']), row.point_name];
}
export function targetOf(row: DeviceSourceTarget): DeviceSourceTarget { return { facility_id: row.facility_id, point_id: row.point_id, channel_id: row.channel_id }; }
export function visibleOutput(source: DeviceSourceCurrent['source']) { return source.configured && source.visibility === 'full' ? source.output ?? undefined : undefined; }
export function bindingCommand(scope: DeviceScope, action: BindingAction, current: DeviceSourceCurrent, reason: string, output?: DeviceOutput): BindingCommand | undefined {
  if (!current.actions[action]) return;
  const base = { request_id: '', reason: reason.trim(), expected_source_revision: current.source_revision, expected_channel_revision: current.channel_revision,
    ...(scope.kind === 'project' ? { expected_link_revision: current.link_revision!, expected_resource_access_revision: current.resource_access_revision!, expected_device_access_revision: current.device_access_revision! } : {}) };
  const source = current.source;
  const next = output ? { output_id: output.id, expected_new_output_revision: output.revision, expected_new_device_revision: output.device_revision } : undefined;
  if (action === 'bind' && next) return { ...base, ...next, action };
  if (!source.configured || source.visibility !== 'full' || !source.output) return;
  const old = { expected_binding_id: source.id, expected_binding_revision: source.revision, expected_old_output_revision: source.output.revision, expected_old_device_revision: source.output.device_revision };
  if (action === 'unbind') return { ...base, ...old, action };
  if (action === 'switch' && next) return { ...base, ...old, ...next, action };
}
const uuid = (v: unknown) => typeof v === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(v) ? v : undefined;
export type BindingSearch = { facility?: string; point?: string; channel?: string; q?: string; state?: BindingRow['state'] };
export function bindingSearch(raw: Record<string, unknown>): BindingSearch {
  return { facility: uuid(raw.facility), point: uuid(raw.point), channel: uuid(raw.channel), q: typeof raw.q === 'string' ? raw.q.slice(0, 200) || undefined : undefined, state: typeof raw.state === 'string' && raw.state !== 'all' && Object.hasOwn(stateLabels, raw.state) ? raw.state as BindingRow['state'] : undefined };
}
