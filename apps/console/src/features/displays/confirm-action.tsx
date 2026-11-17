import { useState } from 'react';
import { Button } from '@infrapro/ui/components/button';
import { AlertDialog, AlertDialogContent, AlertDialogHeader, AlertDialogTitle, AlertDialogDescription, AlertDialogFooter, AlertDialogCancel } from '@infrapro/ui/components/alert-dialog';
import { FormNotice } from '@infrapro/ui/patterns/feedback';

export function ConfirmDisplayAction({ label, description, disabled, action }: { label: string; description: string; disabled?: boolean; action: () => Promise<unknown> }) {
  const [open, setOpen] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState('');
  return <><Button variant="outline" size="sm" disabled={disabled} onClick={() => { setError(''); setOpen(true); }}>{label}</Button><AlertDialog open={open} onOpenChange={value => { if (!busy) setOpen(value); }}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{label}</AlertDialogTitle><AlertDialogDescription>{description}</AlertDialogDescription></AlertDialogHeader><FormNotice error>{error}</FormNotice><AlertDialogFooter><AlertDialogCancel disabled={busy}>取消</AlertDialogCancel><Button variant="destructive" disabled={busy || disabled} onClick={() => { setBusy(true); void action().then(() => setOpen(false)).catch(e => setError(e instanceof Error ? e.message : '操作失败')).finally(() => setBusy(false)); }}>{busy ? '正在处理…' : '确认' + label}</Button></AlertDialogFooter></AlertDialogContent></AlertDialog></>;
}
