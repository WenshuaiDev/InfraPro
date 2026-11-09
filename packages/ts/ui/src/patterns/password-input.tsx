import { useState, type ComponentProps } from 'react';
import { Eye, EyeOff } from 'lucide-react';
import { Input } from '../components/input';
import { Button } from '../components/button';
import { cn } from '../lib/utils';

export function PasswordInput({ className, visibilityLabel = '密码', ...props }: ComponentProps<typeof Input> & { visibilityLabel?: string }) {
  const [visible, setVisible] = useState(false);
  return <div className="relative">
    <Input {...props} type={visible ? 'text' : 'password'} className={cn('pe-10', className)} />
    <Button type="button" variant="ghost" size="icon" className="absolute end-0 top-0 h-9 w-9 hover:bg-transparent" disabled={props.disabled}
      aria-label={`${visible ? '隐藏' : '显示'}${visibilityLabel}`} aria-pressed={visible} onClick={() => setVisible(!visible)}>
      {visible ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
    </Button>
  </div>;
}
