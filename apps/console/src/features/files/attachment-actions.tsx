import { useRef, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import type { FileAttachment, FileAttachmentCommand, FileCommandResult, FileRevision } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Textarea } from '@infrapro/ui/components/textarea';
import { Label } from '@infrapro/ui/components/label';
import { FileState } from '@infrapro/ui/patterns/files';
import { useFiles } from './context';
import { fileAction, type AttachmentCommandKind } from './file-format';

const titles = { update: '编辑信息', trash: '移入回收站', restore: '恢复附件', revert: '还原历史版本' };
const submitLabels = { update: '保存名称与说明', trash: '移入回收站', restore: '恢复附件', revert: '将所选历史正文设为新修订' };

export function AttachmentActionForm({ attachment, kind, selected, onBusy, onDone, onCancel, onRefresh }: {
  attachment: FileAttachment; kind: AttachmentCommandKind; selected?: FileRevision;
  onBusy: (busy: boolean) => void; onDone: (result: FileCommandResult) => void; onCancel: () => void; onRefresh: () => void;
}) {
  const f = useFiles();
  // Keep the original CAS and epoch while editing or replaying a lost response.
  const [snapshot] = useState(() => ({ revision: attachment.revision, epoch: f.capabilities.data?.restore_epoch }));
  const [name, setName] = useState(attachment.display_name), [description, setDescription] = useState(attachment.description), [reason, setReason] = useState('');
  const receipt = useRef<{ key: string; input: FileAttachmentCommand } | null>(null);
  const allowed = !f.capabilities.error && f.capabilities.data?.mode === 'online' && fileAction(attachment, kind);
  const action = useMutation({
    onMutate: () => onBusy(true), onSettled: () => onBusy(false),
    mutationFn: async () => {
      if (!allowed || !snapshot.epoch) throw Error('当前操作权限或空间状态已变化，请重新核对。');
      if (snapshot.epoch !== f.capabilities.data?.restore_epoch) throw Error('文件服务恢复代次已变化，请重新载入附件。');
      const body = { expected_revision: snapshot.revision, reason: reason.trim(), ...(kind === 'update' ? { display_name: name.trim(), description } : {}), ...(kind === 'revert' ? { revision_id: selected?.id } : {}) };
      const key = JSON.stringify({ kind, body });
      if (receipt.current?.key !== key) receipt.current = { key, input: { ...body, request_id: crypto.randomUUID() } };
      return f.client.fileChangeAttachment(f.scope, attachment.id, kind, receipt.current.input, snapshot.epoch);
    },
    onSuccess: async result => { await f.changed(); onDone(result); },
  });
  return <form className="space-y-5" onSubmit={event => { event.preventDefault(); if (reason.trim() && !action.isPending) action.mutate(); }}>
    <div><h3 className="text-base font-semibold">{titles[kind]}</h3>{kind !== 'update' && <p className="mt-2 break-words text-sm leading-6 text-muted-foreground">{kind === 'trash' ? '附件将从普通附件列表移除，保留期间可在回收站恢复。' : kind === 'restore' ? '恢复后可重新查看此附件，已有版本记录会保留。' : `将版本 ${selected?.revision_no ?? ''} 的文件内容设为一个新版本，已有版本记录会保留。`}</p>}</div>
    {kind === 'update' && <><div className="space-y-2"><Label htmlFor="attachment-name">显示名称</Label><Input id="attachment-name" value={name} required maxLength={200} disabled={action.isPending} onChange={event => setName(event.target.value)} /></div><div className="space-y-2"><Label htmlFor="attachment-description">说明</Label><Textarea id="attachment-description" rows={4} value={description} maxLength={1000} disabled={action.isPending} onChange={event => setDescription(event.target.value)} /></div></>}
    <div className="space-y-2"><Label htmlFor="attachment-reason">操作原因</Label><Textarea id="attachment-reason" value={reason} required maxLength={500} rows={2} disabled={action.isPending} onChange={event => setReason(event.target.value)} placeholder="简要说明本次修改的原因" /></div>
    {!allowed && <FileState>当前状态不允许此操作，请返回附件详情核对。</FileState>}
    {attachment.revision !== snapshot.revision && <FileState>附件已更新。本次编辑仍基于打开表单时的版本；请核对后重新载入。</FileState>}
    {action.error && <div className="space-y-2"><FileState error>{action.error.message}</FileState><Button type="button" variant="outline" size="sm" onClick={onRefresh}>重新载入附件</Button></div>}
    <div className="flex flex-wrap gap-2"><Button type="submit" variant={kind === 'trash' ? 'destructive' : 'default'} disabled={!allowed || !reason.trim() || kind === 'update' && !name.trim() || action.isPending || kind === 'revert' && (!selected || selected.id === attachment.current_revision.id || selected.file_state !== 'available')}>{action.isPending ? '正在提交…' : submitLabels[kind]}</Button><Button type="button" variant="outline" disabled={action.isPending} onClick={onCancel}>取消</Button></div>
  </form>;
}
