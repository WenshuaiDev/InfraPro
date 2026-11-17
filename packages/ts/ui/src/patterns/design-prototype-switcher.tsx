// THROWAWAY shared prototype controls. This bar is never rendered in production.
import { useEffect } from 'react';
import { ChevronLeft, ChevronRight, RotateCcw } from 'lucide-react';

export function DesignPrototypeSwitcher({ current, variants, change, reset, dialogShortcuts = false }: { current: string; variants: { key: string; name: string }[]; change: (key: string) => void; reset: () => void; dialogShortcuts?: boolean }) {
  const index = variants.findIndex(variant => variant.key === current);
  useEffect(() => {
    if (process.env.NODE_ENV === 'production') return;
    const listener = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement;
      if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey || target.closest('input, textarea, select, [contenteditable], [role="tree"], [data-prototype-tree]') || (!dialogShortcuts && target.closest('[role="dialog"]'))) return;
      if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
        event.preventDefault();
        change(variants[(index + (event.key === 'ArrowLeft' ? -1 : 1) + variants.length) % variants.length]?.key ?? current);
      }
    };
    // Modal primitives can stop bubbling arrow keys before they reach window.
    window.addEventListener('keydown', listener, dialogShortcuts);
    return () => window.removeEventListener('keydown', listener, dialogShortcuts);
  }, [change, current, dialogShortcuts, index, variants]);
  if (process.env.NODE_ENV === 'production') return null;
  return <nav aria-label="原型方案切换" className="fixed bottom-4 left-1/2 z-40 flex max-w-[calc(100vw-1rem)] -translate-x-1/2 items-center gap-1 rounded-full border border-white/15 bg-slate-900 p-1.5 text-white shadow-xl">
    <button type="button" aria-label="上一个方案" className="flex size-9 shrink-0 items-center justify-center rounded-full hover:bg-white/15 focus-visible:outline-2" onClick={() => change(variants[(index - 1 + variants.length) % variants.length]?.key ?? current)}><ChevronLeft className="size-4" /></button>
    {variants.map(variant => <button type="button" key={variant.key} aria-pressed={current === variant.key} onClick={() => change(variant.key)} className={`min-h-9 whitespace-nowrap rounded-full px-3 text-xs font-medium focus-visible:outline-2 ${current === variant.key ? 'bg-white text-slate-900' : 'text-slate-300 hover:bg-white/10'}`}>{variant.key}<span className="ml-1.5 hidden sm:inline">{variant.name}</span></button>)}
    <button type="button" aria-label="下一个方案" className="flex size-9 shrink-0 items-center justify-center rounded-full hover:bg-white/15 focus-visible:outline-2" onClick={() => change(variants[(index + 1) % variants.length]?.key ?? current)}><ChevronRight className="size-4" /></button>
    <span className="mx-1 h-4 border-l border-white/20" /><button type="button" aria-label="重置原型演示" title="重置原型演示" className="flex size-9 shrink-0 items-center justify-center rounded-full text-slate-300 hover:bg-white/15 focus-visible:outline-2" onClick={reset}><RotateCcw className="size-3.5" /></button>
  </nav>;
}
