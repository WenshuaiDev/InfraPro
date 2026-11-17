import { useEffect } from 'react';
import { Button } from '@infrapro/ui/components/button';

export function EditorSteps<T extends string>({ label, value, items, onChange }: { label: string; value: T; items: readonly { value: T; label: string; hint?: string }[]; onChange: (value: T) => void }) {
  return <nav aria-label={label} className="flex min-w-0 flex-wrap gap-1 rounded-xl border bg-muted/20 p-2">{items.map((item, index) => <Button key={item.value} type="button" variant={value === item.value ? 'secondary' : 'ghost'} className="h-auto min-h-11 flex-1 justify-start gap-2 whitespace-normal px-3 py-2 text-left sm:justify-center" aria-current={value === item.value ? 'step' : undefined} onClick={() => onChange(item.value)}><span className={`flex size-6 shrink-0 items-center justify-center rounded-full text-xs ${value === item.value ? 'bg-primary text-primary-foreground' : 'border text-muted-foreground'}`}>{index + 1}</span><span>{item.label}{item.hint && <span className="block text-xs font-normal text-muted-foreground">{item.hint}</span>}</span></Button>)}</nav>;
}

// Validation paths can point into a different step, collapsed node, or input drawer.
// Reveal all containing editors before CommandError restores focus to the field.
// eslint-disable-next-line react-refresh/only-export-components
export function useIssueReveal(onReveal: (path: string) => void) {
  useEffect(() => {
    const reveal = (event: Event) => onReveal((event as CustomEvent<string>).detail);
    document.addEventListener('derived:reveal-issue', reveal);
    return () => document.removeEventListener('derived:reveal-issue', reveal);
  }, [onReveal]);
}
