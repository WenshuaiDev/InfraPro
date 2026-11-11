// THROWAWAY UI comparison controls; remove when a design is selected.
import { useEffect } from 'react';
import { ArrowLeft, ArrowRight } from 'lucide-react';

export type PrototypeVariant = 'A' | 'B' | 'C';
const variants: PrototypeVariant[] = ['A', 'B', 'C'];
const names = { A: '统一顶栏 · 按空间显示侧栏', B: '双层顶栏 · 横向业务导航', C: '极简顶栏 · 按需导航抽屉' };

export function PrototypeSwitcher({ current, onChange, state }: { current: PrototypeVariant; onChange: (next: PrototypeVariant) => void; state: string }) {
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      const target = event.target;
      if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || (target instanceof Element && target.closest('input, textarea, select, [contenteditable], [role="menu"], [role="dialog"], [role="listbox"], [role="tablist"]'))) return;
      if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
      event.preventDefault();
      onChange(variants[(variants.indexOf(current) + (event.key === 'ArrowRight' ? 1 : 2)) % 3] ?? 'A');
    };
    window.addEventListener('keydown', keydown);
    return () => window.removeEventListener('keydown', keydown);
  }, [current, onChange]);
  return <aside aria-label="原型对比工具" className="fixed inset-x-3 bottom-4 z-40 mx-auto w-fit max-w-[calc(100%-1.5rem)] rounded-2xl bg-slate-950 px-3 py-2 text-white shadow-xl">
    <div className="flex items-center justify-center gap-3">
      <button type="button" aria-label="上一个方案" className="flex size-11 shrink-0 items-center justify-center rounded-lg hover:bg-white/15 focus-visible:outline-2 focus-visible:outline-white" onClick={() => onChange(variants[(variants.indexOf(current) + 2) % 3] ?? 'A')}><ArrowLeft className="size-4" /></button>
      <div className="min-w-0 text-center"><p className="text-[11px] tracking-wider text-slate-300">导航原型 · 仅供比较</p><p className="text-sm font-medium" aria-live="polite">{current} / {names[current]}</p></div>
      <button type="button" aria-label="下一个方案" className="flex size-11 shrink-0 items-center justify-center rounded-lg hover:bg-white/15 focus-visible:outline-2 focus-visible:outline-white" onClick={() => onChange(variants[(variants.indexOf(current) + 1) % 3] ?? 'A')}><ArrowRight className="size-4" /></button>
    </div>
    <p className="max-w-lg border-t border-white/15 pt-1.5 text-center text-[11px] leading-relaxed text-slate-300" data-testid="prototype-state">{state}</p>
  </aside>;
}
