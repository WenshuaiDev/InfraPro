import { useState } from 'react';
import { useIdentity } from '@infrapro/api-client/react';
import { ApiError, type EngineeringVersion } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { useOptionalTenant } from '../tenancy/context';
import { useOperation } from '../account/use-operation';
import { Issues, TextControl } from './controls';
import { VersionPicker } from './reference-picker';
import { useRequestMeta } from './use-request-meta';

export function CopyPublicTemplate({ done }: { done: (id: string) => void }) {
  const { client } = useIdentity();
  const tenant = useOptionalTenant();
  const [source, setSource] = useState<EngineeringVersion>();
  const [code, setCode] = useState('');
  const [reason, setReason] = useState('');
  const meta = useRequestMeta();
  const operation = useOperation({
    mutationFn: () => {
      if (!source || !code.trim() || !reason.trim()) throw new Error('请选择公共模板版本，并填写副本编号和复制原因。');
      const input = { code: code.trim(), source_version: source.id };
      return client.engineeringCopy(
        { kind: 'monitoring-templates', tenant: tenant!.tenant.id },
        { ...input, ...meta('copy', input, reason) },
      );
    },
    onSuccess: (result) => done(result.definition_id),
  });
  return (
    <form
      className="space-y-5"
      onSubmit={(event) => {
        event.preventDefault();
        operation.mutate();
      }}
    >
      <VersionPicker
        kind="monitoring-templates"
        label="公共监测模板"
        value={source?.id ?? ''}
        known={source ? [source] : []}
        onChange={(_, version) => setSource(version)}
      />
      {source && (
        <p className="rounded-lg bg-muted/40 p-3 text-sm">{source.content.description || '该模板未填写说明。'}</p>
      )}
      <TextControl label="副本新编号" value={code} onChange={setCode} hint="小写字母开头，可包含数字和下划线。" />
      <TextControl label="复制原因" value={reason} onChange={setReason} />
      <p className="text-sm text-muted-foreground">复制后在本租户独立编辑草稿，来源版本保留在版本记录中。</p>
      <FormNotice error>{operation.error?.message}</FormNotice>
      {operation.error instanceof ApiError && <Issues issues={operation.error.issues} />}
      <Button type="submit" disabled={operation.isPending || !source || !code.trim() || !reason.trim()}>
        复制到本租户
      </Button>
    </form>
  );
}
