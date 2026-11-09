import type { ReactElement } from 'react';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from '../components/alert-dialog';
import { FormNotice } from './feedback';

export function ConfirmDialog({ trigger, open, onOpenChange, title, description, onConfirm, pending, error, confirmLabel = '确认', destructive = false }: {
  trigger: ReactElement; open: boolean; onOpenChange: (open: boolean) => void; title: string; description: string;
  onConfirm: () => void; pending?: boolean; error?: string; confirmLabel?: string; destructive?: boolean;
}) {
  return <AlertDialog open={open} onOpenChange={(value) => { if (!pending) onOpenChange(value); }}>
    <AlertDialogTrigger asChild>{trigger}</AlertDialogTrigger>
    <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{title}</AlertDialogTitle><AlertDialogDescription>{description}</AlertDialogDescription></AlertDialogHeader>
      <FormNotice error>{error}</FormNotice><AlertDialogFooter><AlertDialogCancel disabled={pending}>取消</AlertDialogCancel><AlertDialogAction disabled={pending} className={destructive ? 'bg-destructive text-white hover:bg-destructive/90' : undefined} onClick={(event) => { event.preventDefault(); onConfirm(); }}>{pending ? '正在处理…' : confirmLabel}</AlertDialogAction></AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>;
}
