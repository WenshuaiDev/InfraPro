// THROWAWAY: preserve AdminLayout; only the account area defaults to its existing collapsed state.
import { useState, type ComponentProps } from 'react';
import { AdminLayout } from './admin-layout';
import { useLayoutStore } from '../state/layout-store';
import { PrototypeNotice } from '../shared/prototype-notice';

type Props = ComponentProps<typeof AdminLayout> & { accountScope: boolean; scope: string };

export function NavigationPrototypeLayout({ accountScope, scope, ...props }: Props) {
  const [accountSidebarOpen, setAccountSidebarOpen] = useState(false);
  const businessSidebarOpen = useLayoutStore((state) => state.sidebarOpen);
  const open = accountScope ? accountSidebarOpen : businessSidebarOpen;
  return <><AdminLayout {...props} sidebarControl={accountScope ? { open: accountSidebarOpen, onOpenChange: setAccountSidebarOpen } : undefined} /><PrototypeNotice state={`${scope} · ${open ? '侧栏展开' : '现有窄栏'} · ${accountScope ? '账号区默认折叠' : '沿用原业务布局'} · 业务只读`} /></>;
}
