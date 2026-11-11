import type { ComponentType, MouseEventHandler, ReactElement, ReactNode } from 'react';

export type Brand = { name: string; description?: string; logo: ReactNode };
export type NavigationItem = { title: string; href: string; icon?: ComponentType<{ className?: string }>; active?: boolean };
export type NavigationGroup = { title: string; items: NavigationItem[] };
export type LinkRenderer = (props: { href: string; children: ReactNode; className?: string; onClick?: MouseEventHandler<HTMLAnchorElement>; 'aria-current'?: 'page' }) => ReactElement;
export type LayoutUser = { displayName: string; username: string };
export type AdminLayoutMode = 'plain' | 'workspace';
export type WorkspaceSwitcherHost = {
  placement: 'header' | 'sidebar';
  nestedInDrawer: boolean;
  ready: boolean;
  onSelected: () => void;
};
export type WorkspaceSwitcherRenderer = (host: WorkspaceSwitcherHost) => ReactNode;
