import { createContext, useCallback, useLayoutEffect, useRef } from 'react';

export const ImportReturnFocus = createContext<(batchID: string) => void>(() => {});

type ReturnRequest = {
  batchID: string;
  listKey: string;
  drawerPath: string;
  drawer: Element | null;
};

export function useImportReturnFocus(
  listKey: string,
  listPath: string,
  pathname: string,
  available: boolean,
  loading: boolean,
  rows: readonly { id: string }[],
) {
  const pending = useRef<ReturnRequest | null>(null);

  useLayoutEffect(() => {
    const cancel = () => { pending.current = null; };
    const onFocus = (event: FocusEvent) => {
      const request = pending.current, target = event.target;
      if (!request || !(target instanceof HTMLElement) || request.drawer?.contains(target)) return;
      if (target !== document.body && target.id !== 'import-search' && target.id !== 'import-' + request.batchID) cancel();
    };
    document.addEventListener('pointerdown', cancel, true);
    document.addEventListener('keydown', cancel, true);
    document.addEventListener('focusin', onFocus, true);
    return () => {
      cancel();
      document.removeEventListener('pointerdown', cancel, true);
      document.removeEventListener('keydown', cancel, true);
      document.removeEventListener('focusin', onFocus, true);
    };
  }, []);

  useLayoutEffect(() => {
    const request = pending.current;
    if (!request) return;
    if (!available || request.listKey !== listKey || (pathname !== listPath && pathname !== request.drawerPath)) {
      pending.current = null;
      return;
    }
    if (pathname !== listPath) return;

    // The route can commit before the portaled drawer releases its focus trap.
    // The list data can also arrive after the drawer has already disappeared.
    const apply = () => {
      if (pending.current !== request || request.drawer?.isConnected) return;
      const row = document.getElementById('import-' + request.batchID);
      const fallback = document.getElementById('import-search');
      const active = document.activeElement;
      if (active && active !== document.body && active !== fallback && active !== row) {
        pending.current = null;
        return;
      }
      if (row?.getClientRects().length && rows.some(batch => batch.id === request.batchID)) {
        row.focus({ preventScroll: true });
        pending.current = null;
      } else {
        if (!active || active === document.body) fallback?.focus({ preventScroll: true });
        if (!loading) pending.current = null;
      }
    };
    const closing = request.drawer?.isConnected ? new MutationObserver(() => {
      if (!request.drawer?.isConnected) {
        closing?.disconnect();
        apply();
      }
    }) : null;
    closing?.observe(document.body, { childList: true, subtree: true });
    const frame = requestAnimationFrame(apply);
    return () => { closing?.disconnect(); cancelAnimationFrame(frame); };
  }, [available, listKey, listPath, loading, pathname, rows]);

  return useCallback((batchID: string) => {
    pending.current = { batchID, listKey, drawerPath: pathname, drawer: document.querySelector('[role="dialog"]') };
  }, [listKey, pathname]);
}
