import { useId, useRef, type FormEvent } from 'react';
import { ApiError, type EngineeringCategory } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@infrapro/ui/components/base-select';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';
import { Label } from '@infrapro/ui/components/label';
import { FormNotice } from '@infrapro/ui/patterns/feedback';
import { useOperation } from '../../account/use-operation';
import { Issues } from '../controls';
import { emptyContent } from '../model';
import { useRequestMeta } from '../use-request-meta';
import { unassigned } from './model';

type Props = { categories: EngineeringCategory[]; defaultCategory?: string; done: (id: string) => void };

export function CreateUnit({ categories, defaultCategory, done }: Props) {
  const { client } = useIdentity();
  const meta = useRequestMeta();
  const id = useId();
  const form = useRef<HTMLFormElement>(null);
  const choices = [{ value: unassigned, label: '待归类' }, ...categories.map(category => ({ value: category.key, label: category.name }))];
  const mutation = useOperation({
    mutationFn: (data: FormData) => {
      const field = (name: string) => String(data.get(name) ?? '').trim();
      const category = field('category');
      const content = { ...emptyContent('units', field('name')), unit: { symbol: field('symbol'), category: category === unassigned ? '' : category } };
      const input = { code: field('code'), content };
      if (!content.name || !field('reason')) throw new Error('请填写单位名称和创建原因。');
      return client.engineeringCreate({ kind: 'units' }, { ...input, ...meta('create', input, field('reason')) });
    },
    onSuccess: result => done(result.definition_id),
  });
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    mutation.mutate(new FormData(event.currentTarget));
  }
  return <form ref={form} onSubmit={submit}>
    <fieldset disabled={mutation.isPending} className="grid gap-4">
      <div className="grid gap-2"><Label htmlFor={id + '-category'}>单位类别</Label>
        <Select name="category" defaultValue={defaultCategory || unassigned} items={choices} modal={false}>
          <SelectTrigger id={id + '-category'} className="w-full"><SelectValue /></SelectTrigger>
          <SelectContent container={form} alignItemWithTrigger={false}>{choices.map(choice =>
            <SelectItem key={choice.value} value={choice.value}>{choice.label}</SelectItem>)}</SelectContent>
        </Select></div>
      <div className="grid gap-2"><Label htmlFor={id + '-name'}>单位名称</Label>
        <Input id={id + '-name'} name="name" required maxLength={200} /></div>
      <div className="grid gap-2"><Label htmlFor={id + '-symbol'}>单位符号</Label><Input id={id + '-symbol'} name="symbol" maxLength={30} /></div>
      <div className="grid gap-2"><Label htmlFor={id + '-code'}>单位编号</Label>
        <Input id={id + '-code'} name="code" required pattern="[a-z][a-z0-9_]{0,63}" maxLength={64}
          title="小写字母开头，可包含数字和下划线。" /></div>
      <div className="grid gap-2"><Label htmlFor={id + '-reason'}>创建原因</Label><Input id={id + '-reason'} name="reason" required /></div>
      <FormNotice error>{mutation.error?.message}</FormNotice>
      {mutation.error instanceof ApiError && <Issues issues={mutation.error.issues} />}
      <div><Button type="submit">创建草稿</Button></div>
    </fieldset>
  </form>;
}
