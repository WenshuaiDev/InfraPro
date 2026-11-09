import { createFileRoute } from '@tanstack/react-router';
import { AccountsPage } from '../../../features/access/accounts-page';
export const Route = createFileRoute('/_authenticated/access/accounts')({ component: AccountsPage });
