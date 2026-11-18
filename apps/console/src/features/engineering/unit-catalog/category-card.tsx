import { ChevronRight } from 'lucide-react';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { Card, CardHeader } from '@infrapro/ui/components/card';
import { ScrollArea } from '@infrapro/ui/components/scroll-area';
import { Separator } from '@infrapro/ui/components/separator';
import type { CategoryGroup } from './model';
import './cards.css';

type Props = {
  group: CategoryGroup;
  searching: boolean;
  enter: () => void;
  selectUnit: (id: string) => void;
};

export function CategoryCard({ group, searching, enter, selectUnit }: Props) {
  const { category, units, matches } = group;
  const count = matches.length === units.length ? units.length + ' 个单位' : '匹配 ' + matches.length + ' / ' + units.length;
  const rows = searching ? matches : [...matches].sort((a, b) =>
    (a.symbol.length || Infinity) - (b.symbol.length || Infinity));

  return (
    <Card className="unit-category-card h-44 min-w-0 gap-0 overflow-hidden py-0 shadow-xs transition-colors hover:border-ring focus-within:border-ring">
      <CardHeader className="block shrink-0 px-1 [container-type:normal]">
        <h3>
          <Button id={'unit-category-' + category.key} type="button" variant="ghost" onClick={enter}
            aria-label={'浏览' + category.name + '，' + count}
            className="h-12 w-full justify-start gap-2 px-3 hover:bg-transparent sm:px-4">
            <span className="mr-auto truncate text-base font-semibold" title={category.name}>{category.name}</span>
            <Badge variant="secondary" className="shrink-0 font-normal tabular-nums">{count}</Badge>
            <ChevronRight className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
          </Button>
        </h3>
      </CardHeader>
      <div className="shrink-0 px-4 sm:px-5"><Separator /></div>
      <ScrollArea type="auto" className="mx-1 my-1 min-h-0 flex-1"
        viewportProps={{ tabIndex: 0, role: 'group', 'aria-label': category.name + '的单位' }}>
        <ul className="px-2 sm:px-3">
          {rows.map(unit => (
            <li key={unit.id}>
              <Button type="button" variant="ghost" onClick={() => selectUnit(unit.id)}
                className="h-auto min-h-7 w-full justify-between gap-3 rounded-sm px-1 py-0.5 text-left font-normal whitespace-normal">
                <span className="min-w-0 break-words">{unit.name}</span>
                <span className="max-w-[45%] shrink-0 break-all text-right font-mono text-secondary-foreground">{unit.symbol || '—'}</span>
              </Button>
            </li>
          ))}
        </ul>
      </ScrollArea>
    </Card>
  );
}
