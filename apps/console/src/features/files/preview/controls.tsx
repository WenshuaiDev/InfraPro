import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { ChevronLeft, ChevronRight, Maximize, Minimize, Minus, Plus, RotateCw, Search } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';

export function PreviewFrame({ children, tools, status, title }: { children: ReactNode; tools: ReactNode; status?: ReactNode; title: string }) {
  const ref = useRef<HTMLDivElement>(null), host = useRef<HTMLDivElement>(null), dialog = useRef<HTMLDialogElement>(null);
  const [surface] = useState(() => document.createElement('div')), [full, setFull] = useState(false), [expanded, setExpanded] = useState(false);
  useLayoutEffect(() => { host.current?.append(surface); return () => surface.remove(); }, [surface]);
  const restore = () => { host.current?.append(surface); dialog.current?.close(); setExpanded(false); requestAnimationFrame(() => ref.current?.querySelector<HTMLButtonElement>('[data-fullscreen-control]')?.focus()); };
  const expand = () => { dialog.current?.append(surface); dialog.current?.showModal(); setExpanded(true); };
  useEffect(() => { const change = () => setFull(document.fullscreenElement === ref.current); document.addEventListener('fullscreenchange', change); return () => { document.removeEventListener('fullscreenchange', change); }; }, []);
  const panel = <div ref={ref} aria-label={`${title}在线预览`} data-document-preview className={`flex min-w-0 flex-col overflow-hidden rounded-lg border bg-background text-foreground ${expanded ? 'h-dvh rounded-none' : 'h-[min(65dvh,44rem)] min-h-80'} [&:fullscreen]:h-dvh [&:fullscreen]:rounded-none [&:fullscreen]:p-3`} onKeyDown={event => { if (event.key === 'Escape' && (full || expanded)) { event.preventDefault(); event.stopPropagation(); if (expanded) restore(); if (document.fullscreenElement) void document.exitFullscreen(); } }}>
    <div className="flex shrink-0 flex-wrap items-center gap-2 border-b bg-muted/30 p-2" aria-label="预览工具栏">
      {tools}<Button data-fullscreen-control size="icon" variant="ghost" className="ml-auto size-10 shrink-0" title={full || expanded ? '退出全屏' : '全屏查看'} aria-label={full || expanded ? '退出全屏' : '全屏查看'} onClick={() => { if (full) void document.exitFullscreen(); else if (expanded) restore(); else if (document.fullscreenEnabled && ref.current?.requestFullscreen) void ref.current.requestFullscreen().catch(expand); else expand(); }}>{full || expanded ? <Minimize /> : <Maximize />}</Button>
    </div>
    <div className="relative min-h-0 min-w-0 flex-1 overflow-hidden">{children}</div>
    {status && <div className="shrink-0 border-t px-3 py-2 text-xs text-muted-foreground" role="status">{status}</div>}
  </div>;
  return <><div ref={host} /><dialog ref={dialog} aria-label={`${title}全屏预览`} className="m-0 h-dvh max-h-none w-screen max-w-none border-0 bg-background p-0 text-foreground backdrop:bg-black/50" onCancel={event => { event.preventDefault(); event.stopPropagation(); restore(); }} />{createPortal(panel, surface)}</>;
}

export function PageControls({ page, count, onPage, unit = '页' }: { page: number; count: number; onPage: (value: number) => void; unit?: string }) {
  const [draft, setDraft] = useState<string>();
  const submit = () => { const value = Number(draft); if (draft !== undefined && Number.isInteger(value) && value >= 1 && value <= count) onPage(value); setDraft(undefined); };
  return <div className="flex items-center gap-1">
    <Button size="icon" variant="ghost" className="size-10" aria-label={`上一${unit}`} disabled={page <= 1} onClick={() => onPage(page - 1)}><ChevronLeft /></Button>
    <Input aria-label={`跳转${unit}码`} className="h-9 w-14 text-center tabular-nums" inputMode="numeric" value={draft ?? page} onChange={event => setDraft(event.target.value)} onBlur={submit} onKeyDown={event => { if (event.key === 'Enter') { submit(); event.currentTarget.blur(); } }} />
    <span className="whitespace-nowrap text-xs tabular-nums">/ {count} {unit}</span>
    <Button size="icon" variant="ghost" className="size-10" aria-label={`下一${unit}`} disabled={page >= count} onClick={() => onPage(page + 1)}><ChevronRight /></Button>
  </div>;
}

export function ZoomControls({ zoom, onZoom, onFit, onRotate }: { zoom: number; onZoom: (zoom: number) => void; onFit?: () => void; onRotate?: () => void }) {
  return <div className="flex items-center gap-1">
    <Button size="icon" variant="ghost" className="size-10" aria-label="缩小" disabled={zoom <= 0.5} onClick={() => onZoom(Math.max(0.5, Math.round((zoom - 0.25) * 100) / 100))}><Minus /></Button>
    <button className="min-h-10 min-w-12 rounded px-1 text-xs tabular-nums focus-visible:outline-2" aria-label="恢复原始比例" onClick={() => onZoom(1)}>{Math.round(zoom * 100)}%</button>
    <Button size="icon" variant="ghost" className="size-10" aria-label="放大" disabled={zoom >= 3} onClick={() => onZoom(Math.min(3, Math.round((zoom + 0.25) * 100) / 100))}><Plus /></Button>
    {onFit && <Button size="sm" variant="ghost" onClick={onFit}>适合宽度</Button>}
    {onRotate && <Button size="icon" variant="ghost" className="size-10" aria-label="顺时针旋转" onClick={onRotate}><RotateCw /></Button>}
  </div>;
}

export function SearchControl({ value, onChange, onNext, onPrevious, count }: { value: string; onChange: (value: string) => void; onNext?: () => void; onPrevious?: () => void; count?: string }) {
  return <div className="flex min-w-0 flex-wrap items-center gap-1">
    <div className="relative min-w-0"><Search aria-hidden="true" className="absolute left-2 top-2.5 size-4 text-muted-foreground" /><Input className="h-9 w-40 pl-8" aria-label="搜索文档" placeholder="搜索内容" value={value} onChange={event => onChange(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') { event.preventDefault(); if (event.shiftKey) onPrevious?.(); else onNext?.(); } }} /></div>
    {onPrevious && <Button size="icon" variant="ghost" className="size-9" aria-label="上一个匹配" disabled={!value} onClick={onPrevious}><ChevronLeft /></Button>}
    {onNext && <Button size="icon" variant="ghost" className="size-9" aria-label="下一个匹配" disabled={!value} onClick={onNext}><ChevronRight /></Button>}
    {value && count && <span className="text-xs tabular-nums" role="status">{count}</span>}
  </div>;
}

export function Highlight({ text, query }: { text: string; query: string }) {
  if (!query) return <>{text}</>;
  const at = text.toLocaleLowerCase().indexOf(query.toLocaleLowerCase());
  if (at < 0) return <>{text}</>;
  return <>{text.slice(0, at)}<mark className="bg-yellow-200 text-black">{text.slice(at, at + query.length)}</mark>{text.slice(at + query.length)}</>;
}
