import { useRef } from 'react';
import type { FileAttachment } from '@infrapro/api-client';
import { Download, Eye, MoreHorizontal, Pencil, RotateCcw, Trash2 } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Badge } from '@infrapro/ui/components/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@infrapro/ui/components/table';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@infrapro/ui/components/dropdown-menu';
import { AttachmentIcon } from './attachment-visual';
import { useFiles } from './context';
import { fileAction, fileContentAllowed, fileDate, fileSize, type AttachmentView } from './file-format';

export function AttachmentsTable({ items, onOpen, layout = 'table' }: { items: FileAttachment[]; onOpen: (file: FileAttachment, view?: AttachmentView) => void; layout?: 'table' | 'list' }) {
  const f = useFiles();
  const openingDrawer = useRef(false);
  const openFromMenu = (file: FileAttachment, view?: AttachmentView) => { openingDrawer.current = true; onOpen(file, view); };
  const actions = (file: FileAttachment) => <DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" size="icon" aria-label={`${file.display_name} 的更多操作`}><MoreHorizontal aria-hidden="true" /></Button></DropdownMenuTrigger><DropdownMenuContent align="end" onCloseAutoFocus={event => { if (openingDrawer.current) { event.preventDefault(); openingDrawer.current = false; } }}>
    <DropdownMenuItem onSelect={() => openFromMenu(file)}><Eye aria-hidden="true" />查看详情</DropdownMenuItem>
    {!f.capabilities.error && fileContentAllowed(f.capabilities.data, file, file.current_revision) && <DropdownMenuItem asChild><a href={f.client.fileDownloadURL(f.scope, file.id, file.current_revision.id)} download><Download aria-hidden="true" />下载原件</a></DropdownMenuItem>}
    {fileAction(file, 'rename') && <><DropdownMenuSeparator /><DropdownMenuItem onSelect={() => openFromMenu(file, 'edit')}><Pencil aria-hidden="true" />编辑信息</DropdownMenuItem></>}
    {fileAction(file, 'trash') && <DropdownMenuItem variant="destructive" onSelect={() => openFromMenu(file, 'trash')}><Trash2 aria-hidden="true" />移入回收站</DropdownMenuItem>}
    {fileAction(file, 'restore') && <DropdownMenuItem onSelect={() => openFromMenu(file, 'restore')}><RotateCcw aria-hidden="true" />恢复附件</DropdownMenuItem>}
  </DropdownMenuContent></DropdownMenu>;
  if (layout === 'list') return <ul aria-label="附件列表" className="divide-y">{items.map(file => <li key={file.id} data-file-row={file.id} className="flex min-w-0 items-center gap-2 py-3 sm:gap-3">
    <button id={`file-open-${file.id}`} aria-label={`查看附件 ${file.display_name}`} className="group flex min-w-0 flex-1 items-center gap-3 rounded py-1 text-left outline-offset-4 focus-visible:outline-2 focus-visible:outline-ring" onClick={() => onOpen(file)}>
      <AttachmentIcon format={file.current_revision.format} />
      <span className="min-w-0"><span className="block break-words text-sm font-medium [overflow-wrap:anywhere] group-hover:underline">{file.display_name}</span><span className="mt-1 flex flex-wrap gap-x-1 text-xs text-muted-foreground"><span>{file.current_revision.format.toUpperCase()}</span><span aria-hidden="true">·</span><span>{fileSize(file.current_revision.size_bytes)}</span><span aria-hidden="true">·</span><span>版本 {file.current_revision.revision_no}</span></span></span>
    </button>
    <time dateTime={file.updated_at} className="hidden shrink-0 text-xs text-muted-foreground tabular-nums sm:block"><span className="sr-only">更新时间 </span>{fileDate(file.updated_at)}</time>
    {actions(file)}
  </li>)}</ul>;
  return <div className="overflow-hidden rounded-lg border"><Table aria-label="附件列表"><TableHeader><TableRow className="bg-muted/30 hover:bg-muted/30">
    <TableHead className="pl-4">文件名称</TableHead><TableHead className="hidden w-24 md:table-cell">类型</TableHead><TableHead className="hidden w-28 sm:table-cell">大小</TableHead><TableHead className="hidden w-40 lg:table-cell">更新时间</TableHead><TableHead className="w-16"><span className="sr-only">操作</span></TableHead>
  </TableRow></TableHeader><TableBody>{items.map(file => <TableRow key={file.id} data-file-row={file.id} className="h-[76px] hover:bg-muted/35">
    <TableCell className="max-w-0 py-3 pl-4"><button id={`file-open-${file.id}`} aria-label={`查看附件 ${file.display_name}`} className="flex max-w-full items-center gap-3 rounded text-left outline-offset-4 focus-visible:outline-2 focus-visible:outline-ring" onClick={() => onOpen(file)}><AttachmentIcon format={file.current_revision.format} /><span className="min-w-0"><span className="block truncate text-sm font-medium">{file.display_name}</span><span className="mt-1 block text-xs text-muted-foreground">{file.status === 'trashed' ? '已移入回收站' : `版本 ${file.current_revision.revision_no}`}<span className="sm:hidden"> · {fileSize(file.current_revision.size_bytes)}</span></span></span></button></TableCell>
    <TableCell className="hidden md:table-cell"><Badge variant="outline" className="font-normal">{file.current_revision.format.toUpperCase()}</Badge></TableCell><TableCell className="hidden text-muted-foreground tabular-nums sm:table-cell">{fileSize(file.current_revision.size_bytes)}</TableCell><TableCell className="hidden text-muted-foreground tabular-nums lg:table-cell">{fileDate(file.updated_at)}</TableCell>
    <TableCell>{actions(file)}</TableCell>
  </TableRow>)}</TableBody></Table></div>;
}
