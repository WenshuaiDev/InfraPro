import type { ReactNode } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { FilesWorkspace } from './workspace';
import { useFiles } from './context';
import type { FileSearch } from './state';

export function AttachmentPanel({ layout = 'table', title, search, onSearch }: {
  layout?: 'table' | 'list';
  title?: ReactNode;
  search?: FileSearch;
  onSearch?: (search: FileSearch) => void;
}) {
  const f = useFiles(), navigate = useNavigate();
  return <div className={layout === 'list' ? 'min-w-0' : 'mt-6'}><FilesWorkspace compact compactTitle={title} attachmentLayout={layout} search={{ ...search, tab: 'active' }} onSearch={onSearch ?? (value => void navigate({ to: f.path, search: value }))} /></div>;
}
