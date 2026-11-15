import { lazy, Suspense, useEffect, useRef } from 'react';
import { useQuery } from '@tanstack/react-query';
import { FileText, ImageIcon, LoaderCircle } from 'lucide-react';
import type { FileAttachment, FileRevision } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { cn } from '@infrapro/ui/lib/utils';
import { useFiles } from './context';
import { fileContentAllowed } from './file-format';
import { fileStates, fileReason } from './state';
import { documentFormats } from './preview/model';

const DocumentPreview = lazy(() => import('./preview/document-preview'));

export function AttachmentIcon({ format, large = false }: { format: string; large?: boolean }) {
  const image = ['png', 'jpeg', 'jpg'].includes(format.toLowerCase()), Icon = image ? ImageIcon : FileText;
  return <span className={cn('inline-flex shrink-0 items-center justify-center rounded-lg border', large ? 'size-14' : 'size-10',
    image ? 'border-violet-200/60 bg-violet-50 text-violet-600 dark:border-violet-800 dark:bg-violet-950 dark:text-violet-300' :
      format.toLowerCase() === 'pdf' ? 'border-rose-200/60 bg-rose-50 text-rose-600 dark:border-rose-800 dark:bg-rose-950 dark:text-rose-300' : 'border-blue-200/60 bg-blue-50 text-blue-600 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-300')}>
    <Icon aria-hidden="true" className={large ? 'size-7 stroke-[1.5]' : 'size-5 stroke-[1.5]'} />
  </span>;
}

export function AttachmentPreview({ attachment, revision }: { attachment: FileAttachment; revision: FileRevision }) {
  const f = useFiles();
  const allowed = !f.capabilities.error && fileContentAllowed(f.capabilities.data, attachment, revision);
  const preview = useQuery({
    queryKey: [...f.prefix, 'preview', attachment.id, revision.id],
    queryFn: ({ signal }) => f.client.filePreview(f.scope, attachment.id, revision.id, signal),
    enabled: allowed && revision.preview_state === 'available' && !documentFormats.includes(revision.format), retry: false, staleTime: 30_000, gcTime: 0,
  });
  if (allowed && documentFormats.includes(revision.format)) return <Suspense fallback={<p role="status" className="p-6 text-sm">正在加载阅读器…</p>}><DocumentPreview attachment={attachment} revision={revision} /></Suspense>;
  let title = '此文件暂不支持在线预览', hint = '可以下载原件，在本地打开。';
  if (!allowed) {
    title = attachment.status === 'trashed' ? '附件已移入回收站' : !f.capabilities.data?.can_download ? '当前可查看文件信息' : '文件内容暂不可用';
    hint = attachment.status === 'trashed' ? '恢复后可按权限查看文件。' : !f.capabilities.data?.can_download ? '下载和预览需要文件下载权限。' : fileStates[revision.file_state] ?? '请稍后重试或联系管理员。';
  } else if (preview.isFetching) { title = '正在加载预览'; hint = '请稍候…'; }
  else if (preview.error) { title = '预览暂时无法加载'; hint = preview.error.message; }
  else if (revision.preview_reason) hint = fileReason(revision.preview_reason);
  return <div className="flex min-h-48 items-center justify-center overflow-hidden rounded-lg border bg-muted/30">
    {allowed && preview.data && !preview.error ? <PreviewImage blob={preview.data} alt={`${attachment.display_name} 修订 ${revision.revision_no} 的安全预览`} /> :
      <div className="flex flex-col items-center gap-3 px-5 py-8 text-center">
        {preview.isFetching && allowed ? <LoaderCircle aria-hidden="true" className="size-7 animate-spin text-muted-foreground" /> : <AttachmentIcon format={revision.format} large />}
        <p className="text-sm font-medium" role={preview.isFetching ? 'status' : undefined}>{title}</p><p className="max-w-sm break-words text-xs leading-5 text-muted-foreground">{hint}</p>
        {allowed && preview.error && <Button variant="outline" size="sm" onClick={() => void preview.refetch()}>重试预览</Button>}
      </div>}
  </div>;
}

function PreviewImage({ blob, alt }: { blob: Blob; alt: string }) {
  const ref = useRef<HTMLImageElement>(null);
  useEffect(() => {
    const image = ref.current, url = URL.createObjectURL(blob);
    if (image) image.src = url;
    return () => { image?.removeAttribute('src'); URL.revokeObjectURL(url); };
  }, [blob]);
  return <img ref={ref} alt={alt} className="max-h-80 w-full object-contain p-4" />;
}
