import { createFileRoute } from '@tanstack/react-router';
import { AccountsPage } from '../../../features/access/accounts-page';
export const Route = createFileRoute('/_authenticated/access/accounts')({ validateSearch:(s:Record<string,unknown>):{account?:string}=>({account:typeof s.account==='string'&&/^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/i.test(s.account)?s.account:undefined}), component: AccountsPage });
