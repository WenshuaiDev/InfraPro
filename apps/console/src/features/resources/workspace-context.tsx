import type { ResourceTreeNode, ResourceTreeRef } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { resourceLabels } from './resource-labels';
import { ResourceIcon, ResourcePagination } from './resource-tree';
import { childFilter, useResourceTree } from './resource-tree-query';
import { ChildrenList, ResourceBreadcrumb } from './workspace-detail';

// A visible ancestor may provide navigation without granting access to its detail.
export function WorkspaceContext({ node, select }: { node: ResourceTreeNode; select: (node: ResourceTreeRef) => void }) {
  const children = useResourceTree(childFilter(node), node.kind !== 'channel');
  return <article className="min-w-0 space-y-5 rounded-lg border bg-card p-4 sm:p-5" aria-label="当前资源详情">
    <ResourceBreadcrumb path={[...node.ancestors, node]} selected={node.id} select={select} />
    <div className="flex min-w-0 items-start gap-3"><ResourceIcon kind={node.kind} /><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><h2 className="break-words text-xl font-semibold">{node.name}</h2><Badge variant="outline">仅路径</Badge></div><p className="mt-1 break-all text-xs text-muted-foreground">{node.code} · {resourceLabels[node.kind]}</p></div></div>
    <FormNotice>仅用于定位已授权资源；未授予完整{resourceLabels[node.kind]}资料。</FormNotice>
    <section className="space-y-3 border-t pt-4">
      <h3 className="text-sm font-semibold">已授权下级资源</h3>
      {children.isPending ? <LoadingState /> : children.error && !children.items.length ? <ErrorState message={children.error.message} onRetry={() => void children.refetch()} /> : <>
        <ChildrenList items={children.items} select={select} empty="暂无可查看的下级资源" />

        <ResourcePagination query={children} />
      </>}
    </section>
  </article>;
}
