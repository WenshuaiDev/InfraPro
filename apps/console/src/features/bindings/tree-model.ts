import type { BindingRow } from './model';

export type BindingBranch = {
  key: string;
  kind: 'facility' | 'component' | 'direct' | 'point';
  name: string;
  code: string;
  children: BindingBranch[];
  channels: BindingRow[];
  count: number;
};

/** Merge complete ancestor paths across cursor pages; names are not identities. */
export function bindingTree(rows: BindingRow[]): BindingBranch[] {
  const roots: BindingBranch[] = [];
  const nodes = new Map<string, BindingBranch>();
  const seen = new Set<string>();
  for (const row of rows) {
    if (seen.has(row.channel_id)) continue;
    seen.add(row.channel_id);
    const path: Pick<BindingBranch, 'key' | 'kind' | 'name' | 'code'>[] = [
      { key: `facility:${row.facility_id}`, kind: 'facility', name: row.facility_name, code: row.facility_code },
      ...(row.component_path.length ? row.component_path.map(component => ({ key: `component:${component.id}`, kind: 'component' as const, name: component.name, code: component.code })) : [{ key: `direct:${row.facility_id}`, kind: 'direct' as const, name: '设施直属（无构件）', code: '' }]),
      { key: `point:${row.point_id}`, kind: 'point', name: row.point_name, code: row.point_code },
    ];
    let siblings = roots;
    for (const [index, part] of path.entries()) {
      let node = nodes.get(part.key);
      if (!node) {
        node = { ...part, children: [], channels: [], count: 0 };
        nodes.set(part.key, node);
        siblings.push(node);
      }
      node.count++;
      if (index === path.length - 1) node.channels.push(row);
      siblings = node.children;
    }
  }
  return roots;
}

export function branchContains(node: BindingBranch, channel?: string): boolean {
  return !!channel && (node.channels.some(row => row.channel_id === channel) || node.children.some(child => branchContains(child, channel)));
}
