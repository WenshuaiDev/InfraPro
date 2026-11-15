// THROWAWAY development helper. The host must gate this component with import.meta.env.DEV.
import { useEffect, type ReactNode } from 'react';
import { ArrowLeft, ArrowRight, Braces } from 'lucide-react';
import { Button } from '../components/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '../components/dialog';

export function PrototypeSwitcher({ enabled, variants, current, onChange, state, children }: {
  enabled: boolean; variants: { key: string; name: string }[]; current: string;
  onChange: (key: string) => void; state: Record<string, unknown>; children?: ReactNode;
}) {
  const index = variants.findIndex(item => item.key === current);
  const stateText = JSON.stringify(state, null, 2);
  useEffect(() => {
    if (!enabled) return;
    // The full relevant state is printed on every variant change (and interaction).
    console.info('[THROWAWAY UI prototype]', stateText);
  }, [enabled, stateText]);
  useEffect(() => {
    if (!enabled) return;
    const handleKey = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey || !['ArrowLeft', 'ArrowRight'].includes(event.key)) return;
      const target = event.target;
      if (target instanceof Element && target.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"]), [role="tablist"], [role="menu"], [role="listbox"]')) return;
      if (document.querySelector('[role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"]')) return;
      event.preventDefault();
      onChange(variants[(index + (event.key === 'ArrowRight' ? 1 : -1) + variants.length) % variants.length]!.key);
    };
    window.addEventListener('keydown', handleKey);
    return () => window.removeEventListener('keydown', handleKey);
  }, [enabled, index, variants, onChange]);
  if (!enabled) return null;
  return <aside aria-label="界面原型切换" className="fixed bottom-4 left-1/2 z-40 flex max-w-[calc(100vw-1.5rem)] -translate-x-1/2 items-center gap-1 rounded-xl border border-slate-700 bg-slate-950 p-1.5 text-slate-50 shadow-xl sm:gap-2 sm:px-3">
    <span className="shrink-0 whitespace-nowrap border-r border-slate-700 pr-2 text-xs text-slate-300 sm:pr-3"><span className="hidden sm:inline">交互</span>原型</span>
    <Button variant="ghost" size="icon" aria-label="上一个原型方案" className="size-9 text-slate-200 hover:bg-slate-800 hover:text-white" onClick={() => onChange(variants[(index - 1 + variants.length) % variants.length]!.key)}><ArrowLeft aria-hidden="true" /></Button>
    <span aria-live="polite" className="min-w-24 whitespace-nowrap text-center text-xs font-medium sm:min-w-32">{current} · {variants[index]?.name}<span className="ml-2 hidden text-slate-400 sm:inline">{index + 1}/{variants.length}</span></span>
    <Button variant="ghost" size="icon" aria-label="下一个原型方案" className="size-9 text-slate-200 hover:bg-slate-800 hover:text-white" onClick={() => onChange(variants[(index + 1) % variants.length]!.key)}><ArrowRight aria-hidden="true" /></Button>
    {children}
    <Dialog><DialogTrigger asChild><Button variant="ghost" size="icon" aria-label="查看原型状态" className="size-9 text-slate-300 hover:bg-slate-800 hover:text-white"><Braces aria-hidden="true" /></Button></DialogTrigger><DialogContent className="max-h-[85svh] overflow-auto sm:max-w-2xl"><DialogHeader><DialogTitle>原型状态</DialogTitle><DialogDescription>当前方案、数据来源、筛选与本地演示操作。刷新页面会清除本地演示操作。</DialogDescription></DialogHeader><pre className="overflow-auto rounded-lg bg-muted p-4 text-xs leading-6">{stateText}</pre></DialogContent></Dialog>
  </aside>;
}
