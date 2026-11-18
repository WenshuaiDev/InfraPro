import { useRef, useState } from 'react';
import { flushSync } from 'react-dom';
import { ApiError, type EngineeringScope, type EngineeringContent, type EngineeringDefinition, type EngineeringIssue, type EngineeringResult, type EngineeringVersion } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { useLivePage } from '../../tenancy/membership-shared';
import { useDefinitionCommands, type DefinitionChange } from '../use-definition-commands';

export const typeScope = { kind: 'type-definitions' } as const;
export type Rights = { editable: boolean; publishable: boolean; stateable: boolean; copyable: boolean };
type Draft = NonNullable<EngineeringDefinition['draft']>;
type Buffer = { base: Draft; revision: number; work: EngineeringContent };
type Check = { snapshot: string; valid: boolean; issues: EngineeringIssue[] };
type PendingResult = { id: string; change: DefinitionChange; result?: EngineeringResult };
type Feedback = { text: string; issues?: EngineeringIssue[]; error?: boolean };
type Options = {
  definition: EngineeringDefinition; rights: Rights; refresh: () => Promise<EngineeringDefinition>;
  copied: (id: string) => void; removed: () => void; scope?: EngineeringScope;
};

const successText = { draft: '已建立草稿。', save: '草稿已保存。', publish: '新版本已发布。', delete: '草稿已删除。', state: '版本使用状态已更新。', copy: '已建立独立定义。' };
function feedback(error: unknown): Feedback {
  return { error: true, text: error instanceof Error ? error.message : '请求失败，请重试。', issues: error instanceof ApiError ? error.issues : undefined };
}

export function useMaintenance({ definition, rights, refresh, copied, removed, scope = typeScope }: Options) {
  const { client } = useIdentity();
  const live = useLivePage();
  const submit = useDefinitionCommands(scope, definition.id);
  const releases = [...definition.versions].sort((a, b) => b.number - a.number);
  const [selection, choose] = useState(definition.draft && (scope.kind !== 'metrics' || !releases.length) ? 'draft' : releases[0]?.id);
  const [buffer, setBuffer] = useState<Buffer>();
  const [pickedUnits, setPickedUnits] = useState<EngineeringVersion[]>([]);
  const [reason, setReason] = useState('');
  const [message, setMessage] = useState<Feedback>();
  const [check, setCheck] = useState<Check>();
  const [pending, setPending] = useState<PendingResult>();
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const base = buffer?.base ?? definition.draft;
  const draft = base ? { ...base, work: buffer?.work ?? base.content } : undefined;
  const isDraft = !!draft && (selection === 'draft' || !releases.length);
  const version = isDraft ? undefined : releases.find((v) => v.id === selection) ?? releases[0];
  const selected = isDraft ? 'draft' : version?.id ?? '';
  const dirty = !!buffer;
  const conflict = !!buffer && (definition.revision !== buffer.revision || definition.draft?.revision !== buffer.base.revision);
  const known = [...definition.dependencies, ...pickedUnits.filter((p) => !definition.dependencies.some((v) => v.id === p.id))];
  const snapshot = [selected, definition.revision, version?.state_revision].join('/');
  const validation = (!dirty || !isDraft) && check?.snapshot === snapshot ? check : undefined;
  const nextNumber = (releases[0]?.number ?? 0) + 1;

  function edit(content: EngineeringContent) {
    if (!rights.editable || !isDraft || !base || lock.current || pending) return;
    setBuffer({ base, revision: buffer?.revision ?? definition.revision, work: content });
    setCheck(undefined);
    setMessage(undefined);
  }
  function select(id: string) {
    if (lock.current || pending) return;
    choose(id);
    setMessage(undefined);
  }
  async function run(action: () => Promise<boolean>) {
    if (lock.current) return false;
    lock.current = true;
    setBusy(true);
    setMessage(undefined);
    try { return await action(); }
    catch (err) { if (live.current) setMessage(feedback(err)); return false; }
    finally { lock.current = false; if (live.current) setBusy(false); }
  }
  async function accept(result: EngineeringResult, change: DefinitionChange) {
    if (change.action === 'copy') { setPending(undefined); copied(result.definition_id); return true; }
    if (change.action === 'delete' && !definition.versions.length) {
      // Removing an unpublished definition also removes its detail URL.
      flushSync(() => { setBuffer(undefined); setPending(undefined); });
      removed();
      return true;
    }
    const fresh = await refresh();
    if (!live.current) return false;
    if (change.action !== 'state') setBuffer(undefined);
    if (change.action === 'draft' || change.action === 'save') choose(fresh.draft ? 'draft' : fresh.versions[0]?.id);
    if (change.action === 'publish') choose(result.version_id);
    if (change.action === 'delete') choose(fresh.versions[0]?.id);
    setCheck(undefined); setPending(undefined);
    setMessage({ text: successText[change.action], issues: result.issues });
    return true;
  }
  async function send(change: DefinitionChange, why: string) {
    let attempt: PendingResult | undefined;
    try {
      const result = await submit(change, why, (id) => { attempt = { id, change }; });
      if (!live.current || !attempt) return false;
      // Keep acknowledged receipts until the authoritative detail has also been read.
      attempt.result = result;
      setPending(attempt);
      return await accept(result, change);
    } catch (err) {
      if (!live.current) return false;
      if (attempt && (attempt.result || !(err instanceof ApiError) || !err.status || err.status >= 500)) {
        setPending(attempt);
        if (attempt.result) { setMessage({ error: true, text: '服务端已完成操作。请重新读取详情。' }); return false; }
      }
      if (err instanceof ApiError && err.status === 409) {
        setCheck(undefined);
        // Preserve the working copy and its original expected revision.
        try { await refresh(); } catch { /* The original conflict remains visible. */ }
      }
      throw err;
    }
  }
  function write(change: DefinitionChange, why = reason) {
    const allowed = change.action === 'copy' ? rights.copyable : change.action === 'publish' ? rights.publishable : change.action === 'state' ? rights.stateable : rights.editable;
    if (!allowed || pending || !why.trim() || (change.action === 'publish' && (!isDraft || dirty || !validation?.valid))) return Promise.resolve(false);
    return run(() => send(change, why.trim()));
  }
  function reconcile() {
    if (!pending) return Promise.resolve(false);
    return run(async () => {
      let result = pending.result;
      if (!result) {
        try { result = await client.engineeringReceipt(scope, pending.id); }
        catch (err) {
          if (live.current && err instanceof ApiError && err.status === 404) {
            setPending(undefined);
            setMessage({ text: '尚未找到这次操作的回执。可以重试，原输入已保留。' });
            return false;
          }
          throw err;
        }
      }
      if (!live.current) return false;
      setPending({ ...pending, result });
      return accept(result, pending.change);
    });
  }
  function reload() {
    return run(async () => {
      const fresh = await refresh();
      if (!live.current) return false;
      setBuffer(undefined); setCheck(undefined);
      choose(selected === 'draft' && fresh.draft ? 'draft' : fresh.versions.find((r) => r.id === selected)?.id ?? (fresh.draft ? 'draft' : fresh.versions[0]?.id));
      setMessage({ text: '已读取最新定义。' }); return true;
    });
  }
  function validate() {
    if ((dirty && isDraft) || pending || !selected) return Promise.resolve(false);
    return run(async () => {
      const result = await client.engineeringPreview(scope, definition.id, { version_id: version?.id ?? '', revision: definition.revision, type_version: '', unit_version: '' });
      if (!live.current) return false;
      setCheck({ snapshot, valid: result.valid, issues: result.issues });
      setMessage({ text: result.valid ? '定义校验通过。' : '定义中有待完善项。', error: !result.valid, issues: result.issues }); return true;
    });
  }
  return {
    scope, definition, releases, selected, version, draft, isDraft, dirty, conflict, known, reason, setReason,
    message, pending, busy, nextNumber, validation, edit, select, write, reconcile, reload, validate,
    save: () => buffer && !conflict ? write({ action: 'save', revision: buffer.revision, content: buffer.work }) : Promise.resolve(false),
    remember: (v: EngineeringVersion) => setPickedUnits((list) => [...list.filter((item) => item.id !== v.id), v]),
  };
}
export type Maintenance = ReturnType<typeof useMaintenance>;
