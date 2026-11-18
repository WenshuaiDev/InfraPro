import { Avatar, AvatarFallback, AvatarImage } from '../components/avatar';
import { cn } from '../lib/utils';

export function AccountAvatar({ name, src, className }: { name: string; src?: string; className?: string }) {
  const initials = Array.from(name.trim()).slice(0, 2).join('').toUpperCase() || '?';
  return <Avatar aria-hidden="true" className={cn('size-9 rounded-lg text-xs', className)}>
    {src && <AvatarImage src={src} alt="" className="object-cover" />}
    <AvatarFallback className="rounded-lg font-medium">{initials}</AvatarFallback>
  </Avatar>;
}
