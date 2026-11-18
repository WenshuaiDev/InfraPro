import { useLayoutEffect, useRef } from 'react';
import { useBlocker } from '@tanstack/react-router';
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@infrapro/ui/components/alert-dialog';

export function UnsavedChanges({ dirty }: { dirty: boolean }) {
  // A save can navigate before the router replaces its registered blocker.
  const committedDirty = useRef(dirty);
  useLayoutEffect(() => { committedDirty.current = dirty; }, [dirty]);
  const blocker = useBlocker({
    shouldBlockFn: () => committedDirty.current,
    withResolver: true,
    enableBeforeUnload: dirty,
  });
  return <AlertDialog open={blocker.status === 'blocked'}>
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>离开尚未保存的编辑？</AlertDialogTitle>
        <AlertDialogDescription>本次未保存的修改会丢失。已保存的草稿仍可在原工作空间继续编辑。</AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel onClick={() => blocker.reset?.()}>继续编辑</AlertDialogCancel>
        <AlertDialogAction onClick={() => blocker.proceed?.()}>放弃修改并离开</AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>;
}
