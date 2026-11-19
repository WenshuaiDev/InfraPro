import { useCallback, useEffect, useState } from 'react';
import { ApiError } from '@infrapro/api-client';

interface Selection<T> {
  item: T;
  command: string;
  trigger: HTMLButtonElement | null;
}

export interface DirectoryDialog {
  command: string;
  trigger: HTMLButtonElement | null;
  paused: boolean;
  close: () => void;
}

// TenantBoundary keys this page by membership and permissions. Keep the selected
// action here so a temporarily hidden table can remove its rows and their portals.
export function useDirectoryAction<T extends { id: string }>(
  items: readonly T[],
  available: boolean,
  list: { isFetching: boolean; isPending: boolean; error: Error | null },
) {
  const [selection, setSelection] = useState<Selection<T>>();
  const select = useCallback((item: T, command: string, trigger: HTMLButtonElement | null) => {
    setSelection({ item, command, trigger });
  }, []);
  const close = useCallback(() => {
    setSelection(current => current === selection ? undefined : current);
  }, [selection]);
  const current = selection && items.find(item => item.id === selection.item.id);
  const denied = list.error instanceof ApiError && [403, 404].includes(list.error.status ?? 0);
  const ready = available && !list.isFetching && !list.isPending && !list.error;
  if (selection && (denied || (ready && !current))) setSelection(undefined);
  const dialog: DirectoryDialog | undefined = selection && {
    command: selection.command,
    trigger: selection.trigger,
    paused: !ready || !current,
    close,
  };
  return { select, item: current ?? selection?.item, dialog };
}

// A changed target may still appear in the list after losing its former action.
export function useDiscardInvalidCommand(dialog: DirectoryDialog | undefined, valid: boolean) {
  const close = dialog?.close, paused = dialog?.paused;
  useEffect(() => { if (close && !paused && !valid) close(); }, [close, paused, valid]);
}
