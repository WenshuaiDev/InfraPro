import type { FileAttachment, FileCapabilities, FileRevision } from '@infrapro/api-client';

export type AttachmentView = 'overview' | 'technical' | 'edit' | 'replace' | 'trash' | 'restore' | 'revert';
export type AttachmentCommandKind = 'update' | 'trash' | 'restore' | 'revert';

export function fileSize(bytes: string | number) {
  const value = Number(bytes);
  if (value < 1024) return `${value} B`;
  if (value < 1024 ** 2) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / 1024 ** 2).toFixed(1)} MB`;
}

export function fileDate(value: string, time = false) {
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit',
    ...(time ? { hour: '2-digit', minute: '2-digit' } as const : {}),
  }).format(new Date(value));
}

export function fileAction(file: FileAttachment, kind: string) {
  return file.allowed_actions.some(action => action.kind === (kind === 'update' ? 'rename' : kind) && action.enabled);
}

export function fileContentAllowed(capabilities: FileCapabilities | undefined, file: FileAttachment, revision: FileRevision) {
  // A historical version can be available even when the current version is missing.
  return !!capabilities?.can_download && capabilities.mode !== 'reconciling' && file.status === 'active' && revision.file_state === 'available';
}
