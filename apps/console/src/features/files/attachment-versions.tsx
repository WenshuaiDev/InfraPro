import type { FileAttachment, FileRevision } from '@infrapro/api-client';
import { ChevronDown, History, RotateCcw } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { fileAction, fileDate, fileSize } from './file-format';
import { fileStates } from './state';

export function AttachmentVersions({ attachment, selected, items, open, onOpenChange, loading, error, hasMore, loadingMore, onMore, onRetry, onSelect, onRestore }: {
  attachment: FileAttachment; selected?: FileRevision; items: FileRevision[]; open: boolean; onOpenChange: (open: boolean) => void;
  loading: boolean; error: Error | null; hasMore: boolean; loadingMore: boolean; onMore: () => void; onRetry: () => void;
  onSelect: (revision: FileRevision) => void; onRestore: () => void;
}) {
  const current = attachment.current_revision;
  // Keep the latest revision available even before the history is requested.
  const versions = [current, ...items.filter(item => item.id !== current.id)];
  const historical = !!selected && selected.id !== current.id;
  return <section aria-label="预览版本" className="space-y-3">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <DropdownMenu open={open} onOpenChange={onOpenChange}>
        <DropdownMenuTrigger asChild><Button variant="outline" className="min-h-11" aria-label="选择预览版本"><History aria-hidden="true" />{selected ? `版本 ${selected.revision_no}` : '选择版本'}{selected && <Badge variant="secondary">{historical ? '历史版本' : '当前版本'}</Badge>}<ChevronDown aria-hidden="true" /></Button></DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="max-h-[min(26rem,var(--radix-dropdown-menu-content-available-height))] w-80 max-w-[calc(100vw-2rem)]" onEscapeKeyDown={event => event.stopPropagation()}>
          <DropdownMenuLabel>选择版本</DropdownMenuLabel>
          <DropdownMenuRadioGroup value={selected?.id ?? ''} onValueChange={value => { const item = versions.find(item => item.id === value); if (item) onSelect(item); }}>
            {versions.map(item => <DropdownMenuRadioItem key={item.id} value={item.id} textValue={`版本 ${item.revision_no}`} aria-label={`版本 ${item.revision_no}${item.id === current.id ? '（当前版本）' : ''}`} className="items-start py-3">
              <span className="min-w-0 flex-1 space-y-1"><span className="flex items-center gap-2 font-medium">版本 {item.revision_no}{item.id === current.id && <Badge variant="secondary">当前版本</Badge>}</span>
                <span className="block text-xs text-muted-foreground">{fileDate(item.created_at, true)} · {fileSize(item.size_bytes)}</span>
                <span className="block line-clamp-2 break-words text-xs leading-5">{item.reason || '更新了附件文件'}{item.reverted_from_revision_id ? ' · 由历史版本还原' : ''}</span>
                {item.file_state !== 'available' && <span className="block text-xs text-destructive">{fileStates[item.file_state] ?? '文件暂不可用'}</span>}
              </span>
            </DropdownMenuRadioItem>)}
          </DropdownMenuRadioGroup>
          {loading && <p role="status" className="px-2 py-3 text-xs text-muted-foreground">正在读取版本记录…</p>}
          {error && <><p role="alert" className="px-2 py-2 text-xs text-destructive">{error.message}</p><DropdownMenuItem onSelect={event => { event.preventDefault(); onRetry(); }}>重新读取版本</DropdownMenuItem></>}
          {hasMore && !error && <><DropdownMenuSeparator /><DropdownMenuItem disabled={loadingMore} onSelect={event => { event.preventDefault(); onMore(); }}>{loadingMore ? '正在加载…' : '加载更多版本'}</DropdownMenuItem></>}
        </DropdownMenuContent>
      </DropdownMenu>
      {historical && <div className="flex flex-wrap gap-2"><Button variant="ghost" size="sm" className="min-h-11" onClick={() => onSelect(current)}>返回最新版</Button>{fileAction(attachment, 'revert') && selected.file_state === 'available' && <Button variant="outline" size="sm" className="min-h-11" onClick={onRestore}><RotateCcw aria-hidden="true" />还原此版本</Button>}</div>}
    </div>
    {selected && <div className="space-y-1 text-xs leading-5 text-muted-foreground"><p>{fileDate(selected.created_at, true)}{selected.reverted_from_revision_id ? ' · 由历史版本还原' : ''}</p>{selected.reason && <p className="whitespace-pre-wrap break-words">{selected.reason}</p>}</div>}
  </section>;
}
