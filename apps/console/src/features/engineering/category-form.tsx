import { useId, useState, type FormEvent } from 'react';
import { LockKeyhole } from 'lucide-react';
import { ApiError, type EngineeringCategory } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { Textarea } from '@infrapro/ui/components/textarea';
import { ConfirmDialog } from '@infrapro/ui/patterns/confirm-dialog';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { Issues } from './controls';
import { useCategoryMutation } from './use-category-mutation';

type Props = {
  category?: EngineeringCategory;
  editable: boolean;
  unitCount?: number;
  done: (key: string, deleted: boolean) => void;
};

export function CategoryForm({ category, editable, unitCount, done }: Props) {
  const id = useId();
  const [reason, setReason] = useState('');
  const [confirm, setConfirm] = useState(false);
  const mutation = useCategoryMutation((key, removed) => {
    setConfirm(false);
    done(key, removed);
  });

  if (category && (!editable || category.used)) {
    return <div className="grid gap-4">
      <div><h3 className="break-words text-lg font-semibold">{category.name}</h3>
        <span className="break-all font-mono text-xs text-muted-foreground">{category.key}</span></div>
      <p className="whitespace-pre-wrap break-words text-sm">{category.description}</p>
      <dl className="grid grid-cols-[6rem_1fr] gap-3 border-y py-4 text-sm [&>dt]:text-muted-foreground">
        {unitCount !== undefined && <><dt>类内单位</dt><dd>{unitCount} 个</dd></>}
        <dt>引用状态</dt><dd>{category.used ? '已引用' : '未引用'}</dd>
      </dl>
      {category.used && <p className="flex items-start gap-2 rounded-md bg-muted p-3 text-sm">
        <LockKeyhole className="mt-0.5 size-4 shrink-0" aria-hidden="true" />类别已被草稿或发布版本引用，含义与编号已锁定。
      </p>}
    </div>;
  }
  if (!editable) return null;

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const field = (name: string) => String(data.get(name) ?? '').trim();
    mutation.mutate({
      category: {
        key: category?.key ?? field('key'), revision: category?.revision ?? 0, used: false,
        name: field('name'), description: field('description'),
      },
      reason,
    });
  }

  return <form onSubmit={submit}>
    <fieldset disabled={mutation.isPending} className="grid gap-4">
      <div className="grid gap-2"><Label htmlFor={id + '-name'}>类别名称</Label>
        <Input id={id + '-name'} name="name" required defaultValue={category?.name} maxLength={100} /></div>
      <div className="grid gap-2"><Label htmlFor={id + '-key'}>类别编号</Label>
        <Input id={id + '-key'} name="key" required defaultValue={category?.key} readOnly={Boolean(category)}
          pattern="[a-z][a-z0-9_]{0,63}" maxLength={64} title="小写字母开头，可包含数字和下划线。" /></div>
      <div className="grid gap-2"><Label htmlFor={id + '-description'}>类别含义</Label>
        <Textarea id={id + '-description'} name="description" required defaultValue={category?.description} maxLength={2000} /></div>
      <div className="grid gap-2"><Label htmlFor={id + '-reason'}>变更原因</Label>
        <Input id={id + '-reason'} required value={reason} onChange={event => setReason(event.target.value)} /></div>
      <FormNotice error>{mutation.error?.message}</FormNotice>
      {mutation.error instanceof ApiError && <Issues issues={mutation.error.issues} />}
      <div className="flex flex-wrap gap-2">
        <Button type="submit">保存类别</Button>
        {category && <ConfirmDialog open={confirm} onOpenChange={setConfirm} destructive
          trigger={<Button type="button" variant="outline" disabled={!reason.trim()}>删除未引用类别</Button>}
          title="删除未引用类别" description={'确认删除「' + category.name + '」？'}
          pending={mutation.isPending} error={mutation.error?.message} onConfirm={() => mutation.mutate({ category, reason, remove: true })} />}
      </div>
    </fieldset>
  </form>;
}
