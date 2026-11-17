import { useState } from 'react';
import { Box, Building2, ChevronDown, ChevronRight, CircleDot, MapPin } from 'lucide-react';
import { cn } from '@infrapro/ui/lib/utils';
import { bindingLocation, type BindingRow } from './model';
import { BindingStatus } from './shared';
import { bindingTree, branchContains, type BindingBranch } from './tree-model';

export function BindingTree({ rows, selected, choose, expandMatches }: { rows: BindingRow[]; selected?: string; choose: (row: BindingRow) => void; expandMatches: boolean }) {
  return <ul aria-label="设施、构件、测点与业务通道" className="min-w-0 space-y-1 p-2">{bindingTree(rows).map(node => <Branch key={node.key} node={node} selected={selected} choose={choose} expandMatches={expandMatches} depth={0} />)}</ul>;
}

function Branch({ node, selected, choose, expandMatches, depth }: { node: BindingBranch; selected?: string; choose: (row: BindingRow) => void; expandMatches: boolean; depth: number }) {
  const containsSelected = branchContains(node, selected);
  const [toggle, setToggle] = useState<{ selected?: string; open: boolean }>();
  const open = toggle && toggle.selected === selected ? toggle.open : expandMatches || containsSelected || node.kind === 'facility' || node.kind === 'point' || node.kind === 'direct';
  const Icon = node.kind === 'facility' ? Building2 : node.kind === 'component' ? Box : MapPin;
  const level = { facility: '设施', component: '构件', point: '测点', direct: '' }[node.kind];
  return <li className="min-w-0">
    <button type="button" className={cn('flex min-h-10 w-full min-w-0 cursor-pointer items-center gap-2 rounded-md px-2 py-2 text-left text-sm outline-none hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring', node.kind === 'facility' && 'font-semibold')} aria-expanded={open} onClick={() => setToggle({ selected, open: !open })} onKeyDown={event => {
      if (event.key === 'ArrowRight' && !open || event.key === 'ArrowLeft' && open) { event.preventDefault(); setToggle({ selected, open: !open }); }
    }}>
      {open ? <ChevronDown className="size-3 shrink-0 text-muted-foreground" aria-hidden="true" /> : <ChevronRight className="size-3 shrink-0 text-muted-foreground" aria-hidden="true" />}
      <Icon className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
      <span className="min-w-0 break-words" title={node.code ? `${node.name} · ${node.code}` : node.name}>{node.name}</span>
      {level && <span className="shrink-0 text-xs font-normal text-muted-foreground">{level}</span>}
      <span className="ml-auto shrink-0 text-xs font-normal tabular-nums text-muted-foreground" title="已加载的业务通道数">{node.count}</span>
    </button>
    {open && <ul className={cn('min-w-0 space-y-1 border-l pl-2', depth < 3 ? 'ml-3' : 'ml-1')}>
      {node.children.map(child => <Branch key={child.key} node={child} selected={selected} choose={choose} expandMatches={expandMatches} depth={depth + 1} />)}
      {node.channels.map(row => <li key={row.channel_id}><button type="button" aria-pressed={row.channel_id === selected} aria-label={`选择 ${[...bindingLocation(row), row.name].join(' / ')}`} data-binding-channel={row.channel_id} className={cn('my-1 flex min-h-14 w-full min-w-0 cursor-pointer items-center gap-2 rounded-md border border-transparent px-2 py-2 text-left outline-none hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring', selected === row.channel_id && 'border-primary/25 bg-primary/5 shadow-[inset_3px_0_0_var(--primary)]')} onClick={() => choose(row)}>
        <CircleDot className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span className="min-w-0 flex-1"><span className="block break-words text-sm font-medium">{row.name}</span><span className="mt-1 block break-words text-xs text-muted-foreground">{row.code} · {row.metric_name}{row.unit_symbol && ` · ${row.unit_symbol}`}</span></span><BindingStatus state={row.state} />
      </button></li>)}
    </ul>}
  </li>;
}
