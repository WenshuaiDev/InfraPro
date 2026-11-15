import { useEffect, useRef, useState } from 'react';

// A menu trigger can disappear when its panel switches to a form. Keep focus
// in the Drawer instead of letting the menu restore it to a detached trigger.
export function useFilePanelView<T extends string>(initial: T) {
  const [view, setView] = useState(initial);
  const contentRef = useRef<HTMLDivElement>(null), menuChangedView = useRef(false);
  useEffect(() => {
    const content = contentRef.current;
    if (!content) return;
    content.parentElement?.scrollTo({ top: 0 });
    const frame = requestAnimationFrame(() => {
      const target = content.querySelector<HTMLElement>('input:not([disabled]), textarea:not([disabled])')
        ?? content.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]')
        ?? content.querySelector<HTMLElement>('button');
      target?.focus({ preventScroll: true });
    });
    return () => cancelAnimationFrame(frame);
  }, [view]);
  const openFromMenu = (next: T) => { menuChangedView.current = next !== view; setView(next); };
  const onMenuCloseAutoFocus = (event: Event) => {
    if (menuChangedView.current) { event.preventDefault(); menuChangedView.current = false; }
  };
  return { view, setView, contentRef, openFromMenu, onMenuCloseAutoFocus };
}
