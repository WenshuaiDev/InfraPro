import { useEffect, useState } from 'react';
import { Search } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@infrapro/ui/components/command';
import type { NavigationGroup } from '../shared/types';

export function CommandSearch({ groups, onNavigate }: { groups: NavigationGroup[]; onNavigate: (href: string) => void }) {
  const [open, setOpen] = useState(false);
  useEffect(() => {
    const listener = (event: KeyboardEvent) => { if (event.key.toLowerCase() === 'k' && (event.metaKey || event.ctrlKey)) { event.preventDefault(); setOpen((value) => !value); } };
    document.addEventListener('keydown', listener);
    return () => document.removeEventListener('keydown', listener);
  }, []);
  return <CommandDialog trigger={<Button variant="outline" className="h-8 justify-start gap-2 text-sm font-normal text-muted-foreground sm:w-56"><Search className="size-4" /><span>搜索页面</span><kbd className="ms-auto hidden rounded border bg-muted px-1 font-mono text-xs sm:inline">⌘ K</kbd></Button>} open={open} onOpenChange={setOpen} title="搜索页面" description="搜索并跳转到已有功能页面。"><CommandInput placeholder="输入页面名称…" /><CommandList><CommandEmpty>没有找到相关页面。</CommandEmpty>{groups.map((group) => <CommandGroup key={group.title} heading={group.title}>{group.items.map((item) => <CommandItem key={item.href} value={item.title} onSelect={() => { setOpen(false); onNavigate(item.href); }}>{item.icon && <item.icon className="size-4" />}{item.title}</CommandItem>)}</CommandGroup>)}</CommandList></CommandDialog>;
}
