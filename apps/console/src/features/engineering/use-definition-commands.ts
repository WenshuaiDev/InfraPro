import { useIdentity } from '@infrapro/api-client/react';
import type { EngineeringContent, EngineeringScope } from '@infrapro/api-client';
import { useRequestMeta } from './use-request-meta';

type DefinitionChange =
  | { action: 'draft'; revision: number; version_id: string }
  | { action: 'save'; revision: number; content: EngineeringContent }
  | { action: 'publish' | 'delete'; revision: number }
  | { action: 'state'; revision: number; version_id: string; enabled: boolean }
  | { action: 'copy'; source_version: string; code: string };

export function useDefinitionCommands(scope: EngineeringScope, id: string) {
  const { client } = useIdentity();
  const meta = useRequestMeta();
  return (change: DefinitionChange, reason: string, onRequest: (id: string) => void) => {
    const trimmed = reason.trim();
    if (!trimmed) throw new Error('请填写本次变更原因。');
    const write = meta(change.action, [scope, id, change], trimmed);
    onRequest(write.request_id);
    switch (change.action) {
      case 'draft': return client.engineeringNewDraft(scope, id, { ...write, revision: change.revision, version_id: change.version_id });
      case 'save': return client.engineeringSave(scope, id, { ...write, revision: change.revision, content: change.content });
      case 'publish': return client.engineeringPublish(scope, id, { ...write, revision: change.revision });
      case 'delete': return client.engineeringDeleteDraft(scope, id, { ...write, revision: change.revision });
      case 'state': return client.engineeringState(scope, id, change.version_id, { ...write, revision: change.revision, enabled: change.enabled });
      case 'copy': return client.engineeringCopy({ ...scope, catalog: false }, { ...write, code: change.code, source_version: change.source_version });
    }
  };
}
export type { DefinitionChange };
