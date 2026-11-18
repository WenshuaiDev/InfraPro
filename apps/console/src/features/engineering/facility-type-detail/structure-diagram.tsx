import { useRef, useState, type ReactNode } from 'react';
import { Box, ChevronRight, Layers, MapPin, ScanLine, Waypoints, type LucideIcon } from 'lucide-react';
import type { EngineeringContent } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { examplePlaces, placeLabels, placeMeanings, type DetailPanel, type ExamplePlace } from './facts';

type ViewProps = { content: EngineeringContent; weather: boolean; inspect: (panel: DetailPanel, target?: string) => void };

const icons: Record<ExamplePlace['kind'] | 'point', LucideIcon> = {
  facility: Layers, component: Box, section: ScanLine, borehole: Waypoints, point: MapPin,
};
type DiagramNode = { id: string; name: string; kind: ExamplePlace['kind'] | 'point'; parent?: string };

export function StructureDiagram({ content, weather, inspect }: ViewProps) {
  const places = examplePlaces(content);
  const root: DiagramNode = places[0]!;
  const firstPart = places.find((place) => place.kind === 'component');
  const branches = places.slice(1).filter((place) => place.kind !== 'component' || place === firstPart);
  const nodes: DiagramNode[] = [
    root, ...branches.map((place) => ({ ...place, parent: root.id })),
    { id: 'point-1', kind: 'point', name: weather ? '站坪测点' : '直属测点', parent: root.id },
  ];
  if (weather || branches.length) nodes.push({
    id: 'point-2', kind: 'point', name: weather ? '风杆顶部测点' : '部位测点', parent: branches[0]?.id ?? root.id,
  });
  const [selection, select] = useState(root.id);
  const inspector = useRef<HTMLElement>(null);
  const tree = useRef<HTMLElement>(null);
  const choose = (node: DiagramNode) => {
    select(node.id);
    if (window.matchMedia('(max-width: 680px)').matches) requestAnimationFrame(() => {
      inspector.current?.focus({ preventScroll: true });
      inspector.current?.scrollIntoView({ block: 'start' });
    });
  };
  const active = nodes.find((node) => node.id === selection) ?? root;
  const activePart = active.kind === 'component' ? content.components?.find((part) => 'component/' + part.key === active.id) : undefined;
  const activeFields = active.kind === 'facility' ? content.fields : activePart?.fields ??
    (active.kind === 'section' ? content.structure?.monitoring_section?.fields : active.kind === 'borehole' ? content.structure?.borehole?.fields : undefined);
  const Icon = icons[active.kind];
  const renderNode = (node: DiagramNode): ReactNode => {
    const Symbol = icons[node.kind];
    const nested = nodes.filter((candidate) => candidate.parent === node.id);
    return <li key={node.id}>
      <button type="button" aria-pressed={active.id === node.id} className="ft-tree-node" onClick={() => choose(node)}>
        <Symbol aria-hidden="true" /><span>{node.name}</span>
      </button>
      {!!nested.length && <ul>{nested.map(renderNode)}</ul>}
    </li>;
  };
  return <section>
    <div className="ft-section-heading"><div><h2>建档结构示意</h2><p>点选一个对象，查看它在工程中代表什么。</p></div>
      <Button variant="ghost" onClick={() => inspect('structure')}>查看全部结构规则<ChevronRight aria-hidden="true" /></Button></div>
    <div className="ft-explorer">
      <nav ref={tree} aria-label="建档结构示例" className="ft-tree">
        <p className="ft-group-title">一种布设示例</p>
        <ul>{renderNode(root)}</ul>
      </nav>
      <article className="ft-inspector" ref={inspector} tabIndex={-1} aria-label="所选对象说明" aria-live="polite">
        <span className="ft-node-icon"><Icon aria-hidden="true" /></span>
        <p className="ft-kind">{active.kind === 'point' ? '测点' : placeLabels[active.kind]}</p>
        <h3>{active.name}</h3><p>{activePart?.description || placeMeanings[active.kind]}</p>
        {active.parent && <dl className="ft-facts"><div><dt>归属位置</dt><dd>{nodes.find((node) => node.id === active.parent)?.name}</dd></div></dl>}
        {active.kind === 'point' && weather && <dl className="ft-facts"><div><dt>通道举例</dt><dd>{active.id === 'point-1' ? '空气温度、空气湿度' : '风速、风向'}</dd></div></dl>}
        {!!activeFields?.length && <p className="ft-node-fields">{activeFields.slice(0, 3).map((f) => f.name).join('、')}{activeFields.length > 3 && '等'}，共 {activeFields.length} 项建档资料。</p>}
        {active.kind !== 'point' && <Button variant="outline" onClick={() => inspect(active.kind === 'facility' ? 'fields' : 'structure', active.id)}>
          {active.kind === 'facility' ? '查看建档资料' : '查看结构规则'}<ChevronRight aria-hidden="true" />
        </Button>}
        <Button className="ft-tree-back" variant="ghost" onClick={() => tree.current?.querySelector<HTMLButtonElement>('[aria-pressed="true"]')?.focus()}>
          返回结构示意
        </Button>
      </article>
    </div>
  </section>;
}
