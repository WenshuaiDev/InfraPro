import { useRef } from 'react';
import { Search, X } from 'lucide-react';
import { Button } from '@infrapro/ui/components/button';
import { Input } from '@infrapro/ui/components/input';

export function DirectorySearch({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  const input = useRef<HTMLInputElement>(null);
  return <div className="relative w-full sm:w-72">
    <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
    <Input ref={input} aria-label={label} placeholder={`${label}…`} value={value} onChange={event => onChange(event.target.value)} className="h-9 pl-9 pr-9 max-sm:h-11 max-sm:pr-11" />
    {value && <Button type="button" variant="ghost" size="icon" className="absolute right-0 top-0 size-9 text-muted-foreground max-sm:size-11" aria-label="清空搜索" onClick={() => { onChange(''); input.current?.focus(); }}><X aria-hidden="true" className="size-4" /></Button>}
  </div>;
}
