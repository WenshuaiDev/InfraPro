import { useState } from 'react';
import { Archive, Trash2 } from 'lucide-react';
import { ApiError, type AnalysisArtifactDetail } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@infrapro/ui/components/alert-dialog';
import { useRequestMeta } from '../engineering/use-request-meta';
import { useOperation } from '../account/use-operation';
import { CommandError, TextField, useDerived } from './common';

type Artifact = AnalysisArtifactDetail['artifact'];
export function ArtifactManagement({artifact, refresh, onDeleted}: {
  artifact: Artifact; refresh: () => Promise<unknown>; onDeleted: (name: string) => Promise<void>;
}) {
  const m = useDerived(), meta = useRequestMeta();
  const [target, setTarget] = useState<{action: 'archive' | 'delete'; artifact: Artifact}>();
  const [reason, setReason] = useState('');
  const command = useOperation({mutationFn: async () => {
    if (!target) return;
    const {action, artifact: original} = target;
    const body = {expected_revision: original.revision};
    const input = {...body, ...meta(action + original.id, body, reason.trim())};
    if (action === 'delete') await m.client.analysisDeleteArtifact(m.scope, original.id, input);
    else await m.client.analysisArchiveArtifact(m.scope, original.id, input);
    return target;
  }, onSuccess: async result => {
    setTarget(undefined);
    if (result?.action === 'delete') await onDeleted(result.artifact.name);
    else await refresh();
  }});
  if (!m.editable || !m.allows('analysis.manage') || !m.allows('analysis.results.read')) return null;
  const open = (action: 'archive' | 'delete') => {
    setReason(''); command.reset(); setTarget({action, artifact: {...artifact}});
  };
  const deleting = target?.action === 'delete';
  const conflict = command.error instanceof ApiError && command.error.status === 409;
  return <div className="space-y-3">
    <div className="flex flex-wrap gap-2">
      {artifact.status === 'active' && <Button variant="outline" onClick={() => open('archive')}><Archive/>归档成果</Button>}
      <Button variant="outline" className="text-destructive" onClick={() => open('delete')}><Trash2/>删除成果</Button>
    </div>
    <AlertDialog open={!!target} onOpenChange={v => { if (!v && !command.isPending) setTarget(undefined); }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{deleting ? '删除整份成果？' : '归档这份成果？'}</AlertDialogTitle>
        <AlertDialogDescription>“{target?.artifact.name}”共有 {target?.artifact.latest_version} 个固定版本。
          {deleting ? '删除后，所有版本和已有的导出链接都将无法访问。原始观测和分析方案保留。' : '归档后停止追加版本，已有成果仍可查看、比较和导出。'}
        </AlertDialogDescription></AlertDialogHeader>
        <TextField label={deleting ? '删除原因' : '归档原因'} value={reason} onChange={setReason} disabled={command.isPending}/>
        <CommandError error={command.error}/>
        {conflict && <p className="text-sm">成果已更新。请重新核对版本后确认。</p>}
        <AlertDialogFooter><AlertDialogCancel disabled={command.isPending}>取消</AlertDialogCancel>
          {conflict ? <Button onClick={async () => { await refresh(); setTarget(undefined); }}>刷新成果</Button> :
            <Button variant={deleting ? 'destructive' : 'default'} disabled={!reason.trim() || command.isPending} onClick={() => command.mutate()}>
              {command.isPending ? '正在提交…' : deleting ? '确认删除全部版本' : '确认归档成果'}
            </Button>}
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>;
}
