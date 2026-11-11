// THROWAWAY review label. It is mounted only in development prototype mode.
export function PrototypeNotice({ state }: { state: string }) {
  return <aside aria-label="导航调整原型" className="fixed inset-x-3 bottom-4 z-40 mx-auto w-fit max-w-[calc(100%-1.5rem)] rounded-xl bg-slate-950 px-5 py-3 text-center text-white shadow-xl"><p className="text-sm font-medium">保留现有布局 · 账号区默认窄栏</p><p data-testid="prototype-state" aria-live="polite" className="mt-1 text-xs leading-relaxed text-slate-300">{state}</p></aside>;
}
