import { createFileRoute, redirect } from '@tanstack/react-router';
import { MyTenantsPage } from '../../features/tenancy/pages';
export const Route = createFileRoute('/_authenticated/tenants')({ beforeLoad: ({ search }) => { if (import.meta.env.DEV && import.meta.env.VITE_NAVIGATION_PROTOTYPE === '1') throw redirect({ to: '/', search: { variant: search.variant ?? 'A', picker: 'tenant' }, replace: true }); }, component: MyTenantsPage });
