import { useRef, type ReactNode, type RefObject } from 'react';
import { X } from 'lucide-react';
import { useIsMobile } from '../hooks/use-mobile';
import { Button } from '../components/button';
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle, DrawerDescription, DrawerClose } from '../components/drawer';

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  children: ReactNode;
  returnFocus?: RefObject<HTMLElement | null>;
  fallbackFocusSelector?: string;
};

export function ResponsivePanel({ open, onOpenChange, title, description, children, returnFocus, fallbackFocusSelector }: Props) {
  const mobile = useIsMobile();
  const previous = useRef<HTMLElement | null>(null);
  const popup = useRef<HTMLDivElement | null>(null);
  const captureFocus = () => { previous.current = document.activeElement instanceof HTMLElement ? document.activeElement : null; return true; };
  const focusTarget = () => {
    let target = returnFocus?.current ?? previous.current;
    // A refreshed table can replace the trigger while its panel is open.
    if (!target?.isConnected && target?.id) target = document.getElementById(target.id);
    if ((!target?.isConnected || target === document.body) && fallbackFocusSelector) target = document.querySelector<HTMLElement>(fallbackFocusSelector);
    return target?.isConnected ? target : null;
  };
  const restoreFocus = () => {
    const target = focusTarget();
    // Base UI resolves finalFocus during cleanup, before React replaces table cells.
    // Resolve again after that commit only when the original target was detached.
    queueMicrotask(() => {
      if (target && !target.isConnected && !popup.current?.hasAttribute('data-open')) focusTarget()?.focus({ preventScroll: true });
    });
    return target ?? true;
  };
  return <Drawer open={open} onOpenChange={onOpenChange} swipeDirection={mobile ? 'down' : 'right'} showSwipeHandle={mobile}>
    <DrawerContent ref={popup} className="bg-background shadow-xl [--drawer-bleed-background:transparent] data-[swipe-axis=x]:h-[calc(100dvh-1.5rem)] data-[swipe-axis=x]:w-[min(35rem,calc(100vw-1.5rem))] data-[swipe-axis=x]:[--drawer-inset:0.75rem] data-[swipe-direction=right]:rounded-2xl data-[swipe-direction=right]:border data-[swipe-axis=y]:h-[92dvh] data-[swipe-axis=y]:max-h-[92dvh] data-[swipe-direction=down]:rounded-t-2xl" initialFocus={captureFocus} finalFocus={restoreFocus}>
      <DrawerHeader className="relative border-b px-5 py-5 pr-16 group-data-[swipe-axis=y]/drawer-popup:text-left sm:px-6 sm:pr-16">
        <DrawerTitle className="break-words text-lg font-semibold">{title}</DrawerTitle>
        <DrawerDescription>{description}</DrawerDescription>
        <DrawerClose render={<Button variant="ghost" size="icon" className="absolute right-3 top-3 size-11" aria-label="关闭面板" />}><X aria-hidden="true" /></DrawerClose>
      </DrawerHeader>
      <div data-slot="drawer-scroll-area" className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 py-5 pb-[max(1.25rem,env(safe-area-inset-bottom))] sm:p-6">{children}</div>
    </DrawerContent>
  </Drawer>;
}
