import type { ComponentProps, ReactNode } from 'react';
import type { Control, FieldPath, FieldValues } from 'react-hook-form';
import { FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from '../components/form';
import { Input } from '../components/input';
import { PasswordInput } from './password-input';

type Props<T extends FieldValues> = Omit<ComponentProps<typeof Input>, 'name' | 'value' | 'onChange' | 'defaultValue'> & {
  control: Control<T>; name: FieldPath<T>; label: string; hint?: ReactNode; labelAction?: ReactNode;
};

export function FormInput<T extends FieldValues>({ control, name, label, hint, labelAction, type, ...props }: Props<T>) {
  return <FormField control={control} name={name} render={({ field }) => <FormItem>
    <div className="flex items-center justify-between gap-2"><FormLabel>{label}</FormLabel>{labelAction}</div>
    <FormControl>{type === 'password' ? <PasswordInput {...props} {...field} visibilityLabel={label} /> : <Input {...props} {...field} type={type} />}</FormControl>
    {hint && <FormDescription>{hint}</FormDescription>}
    <FormMessage role="alert" />
  </FormItem>} />;
}
