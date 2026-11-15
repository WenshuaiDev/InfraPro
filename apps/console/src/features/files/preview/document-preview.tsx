import { Component, lazy, Suspense, useState, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { FileAttachment, FileRevision } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { useFiles } from '../context';
import { maxPreviewBytes } from './model';

const PdfPreview = lazy(() => import('./pdf-preview'));
const DocxPreview = lazy(() => import('./docx-preview'));
const DataPreview = lazy(() => import('./data-preview'));

export default function DocumentPreview({ attachment, revision }: { attachment: FileAttachment; revision: FileRevision }) {
  const f = useFiles(), [attempt, setAttempt] = useState(0);
  const content = useQuery({
    queryKey: [...f.prefix, 'document-content', attachment.id, revision.id, attempt],
    queryFn: async ({ signal }) => {
      if (Number(revision.size_bytes) > maxPreviewBytes) throw Error('文件超过 100 MiB 在线预览上限，请下载原件查看。');
      const data = await f.client.fileContent(f.scope, attachment.id, revision.id, signal);
      if (data.size !== Number(revision.size_bytes) || data.size > maxPreviewBytes) throw Error('文件内容长度不一致，请重新打开附件。');
      return data;
    },
    retry: false, gcTime: 0, staleTime: Infinity, refetchOnWindowFocus: false,
  });
  if (content.error) return <div className="space-y-3 rounded-lg border bg-muted/20 p-6"><p role="alert" className="text-sm">{content.error.message}</p><Button variant="outline" size="sm" onClick={() => setAttempt(value => value + 1)}>重试预览</Button></div>;
  const loading = <div className="flex min-h-60 items-center justify-center rounded-lg border bg-muted/20 p-6 text-sm text-muted-foreground" role="status">正在加载预览…</div>;
  if (!content.data) return loading;
  const props = { blob: content.data, title: attachment.display_name, onRetry: () => setAttempt(value => value + 1) };
  return <ReaderBoundary key={attempt} retry={() => setAttempt(value => value + 1)}><Suspense fallback={loading}>{revision.format === 'pdf' ? <PdfPreview {...props} /> : revision.format === 'docx' ? <DocxPreview {...props} /> : <DataPreview {...props} format={revision.format} />}</Suspense></ReaderBoundary>;
}

class ReaderBoundary extends Component<{ children: ReactNode; retry: () => void }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  render() { return this.state.failed ? <div className="space-y-3 rounded-lg border p-6"><p role="alert" className="text-sm">阅读器加载失败，请重试或下载原件查看。</p><Button variant="outline" onClick={this.props.retry}>重试预览</Button></div> : this.props.children; }
}
