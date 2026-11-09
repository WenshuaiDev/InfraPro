import { z } from '../../config/validation';
import type { FieldValues, Path, UseFormReturn } from 'react-hook-form';
import { ApiError } from '@infrapro/api-client';

export const loginSearchSchema = z.object({ returnTo: z.string().optional().catch(undefined), notice: z.enum(['created', 'password']).optional().catch(undefined) });
export const loginSchema = z.object({ username: z.string().min(1, '请输入用户名。'), password: z.string().min(1, '请输入密码。') });
export function passwordSchema(min = 8, max = 128) {
  return z.string().refine((value) => { const length = Array.from(value).length; return length >= min && length <= max; }, `请输入 ${min}–${max} 个字符的密码。`);
}
export function setFormError<T extends FieldValues>(form: UseFormReturn<T>, error: unknown) {
  form.setError('root.server', { message: error instanceof Error ? error.message : '操作失败，请重试。' });
  if (error instanceof ApiError) for (const [name, message] of Object.entries(error.fields)) {
    if (Object.hasOwn(form.getValues(), name)) form.setError(name as Path<T>, { message });
  }
}
export function date(value: string) { return new Date(value).toLocaleString('zh-CN', { hour12: false }); }
