import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useEffect, useRef, useState, type KeyboardEvent } from 'react';
import type { ResourceKind, ResourceTreeRef, ResourceTreeNode } from '@infrapro/api-client';
import { Blocks, Box, ChevronDown, ChevronRight, ChevronsDownUp, CircleDot, FolderTree, LocateFixed, Radio } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { cn } from '@infrapro/ui/lib/utils';
import { EmptyState, ErrorState, LoadingState } from '@infrapro/ui/patterns/feedback';
import { childFilter, useResourceTree } from './resource-tree-query';
import { resourceLabels } from './resource-labels';
import { useResources } from './context';

const icons = { facility: Blocks, component: Box, monitoring_section: FolderTree, borehole: LocateFixed, point: CircleDot, channel: Radio };
export function ResourceIcon({ kind, small = false }: { kind: ResourceKind; small?: boolean }) {
  const Icon = icons[kind];
  return <span className={cn('inline-flex shrink-0 items-center justify-center text-muted-foreground', small ? 'size-5' : 'size-10 rounded-lg border bg-muted/40')}><Icon className={small ? 'size-4' : 'size-5'} aria-hidden="true" /></span>;
}
type NavNode = ResourceTreeRef & Partial<Pick<ResourceTreeNode, 'has_children' | 'type_name'>>;
type Query = ReturnType<typeof useResourceTree>;
type Props = { query: Query; searching: boolean; filtered: boolean; selected?: string; path: ResourceTreeRef[]; select: (node: ResourceTreeRef) => void; expandOnSelect?: boolean; title?: string; leafKind?: "point" };
type BranchProps = Omit<Props, 'query' | 'filtered'> & { node: NavNode; catalog: NavNode[]; level: number; expanded: Set<string>; toggle: (id: string) => void; focusId?: string; onFocus: (id: string) => void };
function unique(nodes: NavNode[]) {
  const found = new Map<string, NavNode>();
  for (const node of nodes) found.set(node.id, { ...found.get(node.id), ...node });
  return [...found.values()];
}

export function ResourceTree({ query, searching, filtered, selected, path, select, expandOnSelect, title = '资源目录', leafKind }: Props) {
  const { scope } = useResources();
  const tree = useRef<HTMLDivElement>(null);
  const [expanded, setExpanded] = useState(new Set<string>());
  const [focusId, setFocusId] = useState<string>();
  const [locateVersion, setLocateVersion] = useState(0);
  const locating = useRef<string | undefined>(undefined);
  const [expandedStamp, setExpandedStamp] = useState('');
  // Search results contain only matching objects and the paths needed to reach them.
  const catalog = searching ? unique(query.items.flatMap((node) => [...node.ancestors, node])) : unique([...query.items, ...path]);
  const roots = catalog.filter((node) => node.kind === 'facility');
  const pathStamp = path.map((node) => node.id).join('/');
  const resultPaths = searching ? query.items.flatMap((node) => node.ancestors.map((p) => p.id)).join('/') : '';
  const stamp = `${pathStamp}:${resultPaths}`;
  if (stamp !== expandedStamp) {
    setExpandedStamp(stamp);
    setExpanded(new Set([...expanded, ...pathStamp.split('/').filter(Boolean), ...resultPaths.split('/').filter(Boolean)]));
  }
  useEffect(() => {
    const container = tree.current;
    if (!container || !locating.current) return;
    if (locating.current !== selected) { locating.current = undefined; return; }
    const focus = () => {
      const node = container.querySelector<HTMLElement>(`[data-node="${locating.current}"]`);
      if (!node) return false;
      node.focus(); node.scrollIntoView({ block: 'nearest' });
      locating.current = undefined;
      return true;
    };
    if (focus()) return;
    // A cleared filter or lazy branch can finish rendering after the locate click.
    const observer = new MutationObserver(() => { if (focus()) observer.disconnect(); });
    observer.observe(container, { childList: true, subtree: true });
    return () => observer.disconnect();
  }, [locateVersion, selected]);
  const toggle = (id: string) => { setFocusId(id); setExpanded((old) => { const next = new Set(old); if (next.has(id)) next.delete(id); else next.add(id); return next; }); };
  const keyboard = (event: KeyboardEvent<HTMLDivElement>) => {
    const target = event.target as HTMLElement;
    if (target.getAttribute('role') !== 'treeitem') return;
    const rows = [...event.currentTarget.querySelectorAll<HTMLElement>('[role="treeitem"]')];
    const index = rows.indexOf(target);
    let next: HTMLElement | undefined;
    if (event.key === 'ArrowDown') next = rows[Math.min(rows.length - 1, index + 1)];
    if (event.key === 'ArrowUp') next = rows[Math.max(0, index - 1)];
    if (event.key === 'Home') next = rows[0];
    if (event.key === 'End') next = rows.at(-1);
    if (event.key === 'ArrowRight') {
      if (target.getAttribute('aria-expanded') === 'false') toggle(target.dataset.node!);
      else if (target.getAttribute('aria-expanded') === 'true') next = rows[index + 1];
    }
    if (event.key === 'ArrowLeft') {
      if (target.getAttribute('aria-expanded') === 'true') toggle(target.dataset.node!);
      else next = rows.find((row) => row.dataset.node === target.dataset.parent);
    }
    if (event.key === 'Enter' || event.key === ' ') target.querySelector<HTMLElement>('[data-select-resource]')?.click();
    if (['ArrowDown', 'ArrowUp', 'ArrowLeft', 'ArrowRight', 'Home', 'End', 'Enter', ' '].includes(event.key)) { event.preventDefault(); event.stopPropagation(); }
    next?.focus();
  };
  return <>
    <div className="flex h-14 shrink-0 items-center gap-2 border-b px-3">
      <FolderTree className="size-4 text-muted-foreground" aria-hidden="true" /><h2 className="flex-1 text-sm font-semibold">{title}</h2>
      <Button variant="ghost" size="icon" className="size-9" aria-label="定位当前资源" disabled={!path.length} onClick={() => { setExpanded(new Set([...expanded, ...path.map((n) => n.id)])); locating.current = selected; setLocateVersion((version) => version + 1); }}><LocateFixed className="size-4" /></Button>
      <Button variant="ghost" size="icon" className="size-9" aria-label="收起全部资源" onClick={() => { setExpanded(new Set()); setFocusId(roots[0]?.id); }}><ChevronsDownUp className="size-4" /></Button>
    </div>
    <div ref={tree} className="min-h-0 flex-1 overflow-y-auto p-2" data-resource-tree-scroll>
      {query.isPending ? <LoadingState label="正在读取资源目录…" /> : query.error && !query.items.length ? <ErrorState message={query.error.message} onRetry={() => void query.refetch()} /> : !roots.length ? <EmptyState title={filtered ? '没有符合条件的资源' : scope.kind === 'project' ? '当前项目暂无可用设施' : '尚未创建设施'} description={filtered ? '调整搜索或筛选条件后再试。' : scope.kind === 'project' ? '请联系租户授权人员开放设施，或按已有权限新建设施。' : '创建设施后，可以逐层添加构件、测点和通道。'} /> : <div role="tree" aria-label={leafKind ? "设施、构件与测点" : "设施、构件、测点与通道"} onKeyDown={keyboard}>
        {roots.map((node) => <Branch key={node.id} node={node} level={1} catalog={catalog} expanded={expanded} toggle={toggle} selected={selected} focusId={focusId ?? roots[0]?.id} onFocus={setFocusId} path={path} searching={searching} select={select} expandOnSelect={expandOnSelect} leafKind={leafKind} />)}
      </div>}
      <ResourcePagination query={query} label={searching ? '匹配资源' : '设施'} />
    </div>
    <div className="flex shrink-0 flex-wrap gap-x-3 gap-y-2 border-t bg-muted/20 px-3 py-3 text-xs text-muted-foreground">{(Object.keys(resourceLabels) as ResourceKind[]).filter(kind => !leafKind || kind !== "channel").map((kind) => <span key={kind} className="flex items-center gap-0.5"><ResourceIcon kind={kind} small />{resourceLabels[kind]}</span>)}</div>
  </>;
}

function Branch({ node, catalog, level, expanded, toggle, selected, focusId, onFocus, path, searching, select, expandOnSelect, leafKind }: BranchProps) {
  const open = expanded.has(node.id);
  const query = useResourceTree(childFilter(node), open && !searching && !node.deleted_at && node.kind !== 'channel' && node.kind !== leafKind);
  const seeded = (searching ? catalog : path).filter((item) => item.parent_id === node.id);
  const children = searching ? seeded : unique([...query.items, ...seeded]);
  const hasChildren = !node.deleted_at && node.kind !== 'channel' && node.kind !== leafKind && (searching ? children.length > 0 : query.summary ? query.summary.total > 0 : node.has_children !== false || seeded.length > 0);
  return <div role="treeitem" aria-level={level} aria-selected={selected === node.id} aria-expanded={hasChildren ? open : undefined} aria-label={`${node.name}，${resourceLabels[node.kind]}`} data-node={node.id} data-parent={node.parent_id ?? ''} tabIndex={focusId === node.id ? 0 : -1} onFocus={(event) => { if (event.target === event.currentTarget) onFocus(node.id); }} className="rounded-md outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring">
    <div data-select-resource onClick={() => { onFocus(node.id); if (expandOnSelect && hasChildren) toggle(node.id); select(node); }} style={{ paddingLeft: Math.min(level - 1, 6) * 14 + 4 }} className={cn('my-0.5 flex min-h-11 cursor-pointer items-center gap-1 rounded-md pr-2 text-sm transition-colors hover:bg-muted', node.kind === 'facility' && 'mt-1 min-h-12 font-medium', selected === node.id && 'bg-accent font-medium text-accent-foreground')}>
      <button type="button" tabIndex={-1} disabled={!hasChildren} aria-label={`${open ? '收起' : '展开'}${node.name}`} className={cn('flex size-7 shrink-0 items-center justify-center rounded hover:bg-foreground/5', !hasChildren && 'invisible')} onClick={(event) => { event.stopPropagation(); toggle(node.id); }}>{open ? <ChevronDown className="size-3.5" aria-hidden="true" /> : <ChevronRight className="size-3.5" aria-hidden="true" />}</button>
      <ResourceIcon kind={node.kind} small /><span className="min-w-0 flex-1 truncate" title={`${node.name} · ${node.code}`}>{node.name}</span>
      {node.deleted_at || node.status === 'retired' ? <span className="text-xs font-normal text-muted-foreground">{node.deleted_at ? '已删除' : '退役'}</span> : node.kind === 'facility' && node.type_name && !leafKind ? <span className="max-w-16 truncate text-xs font-normal text-muted-foreground" title={node.type_name}>{node.type_name}</span> : null}
    </div>
    {open && hasChildren && <div role="group">
      {!searching && query.error && !query.items.length ? <div className="px-4 py-2"><p role="alert" className="text-xs text-destructive">{query.error.message}</p><Button size="sm" variant="ghost" onClick={() => void query.refetch()}>重试下级目录</Button></div> : <>
        {children.map((child) => <Branch key={child.id} node={child} catalog={catalog} level={level + 1} expanded={expanded} toggle={toggle} selected={selected} focusId={focusId} onFocus={onFocus} path={path} searching={searching} select={select} expandOnSelect={expandOnSelect} leafKind={leafKind} />)}
        {!searching && query.isPending && <p role="status" className="px-5 py-2 text-xs text-muted-foreground">正在读取下级…</p>}
        {!searching && <ResourcePagination query={query} nested label={`${node.name}的下级`} />}
      </>}
    </div>}
  </div>;
}

export function ResourcePagination({ query, label = '下级资源', nested = false }: { query: Query; label?: string; nested?: boolean }) {
  if (!query.data) return null;
  const { page, hasNext, pageSize } = query.pagination;
  return <>{query.error && <p role="alert" className="text-sm text-destructive">{query.error.message}</p>}
    {(!nested || page > 0 || hasNext || pageSize !== 25) && <ListPagination label={label} {...query.pagination} />}</>;
}
