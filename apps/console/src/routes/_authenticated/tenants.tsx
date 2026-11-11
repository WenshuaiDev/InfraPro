import { createFileRoute, redirect } from '@tanstack/react-router';
export const Route = createFileRoute('/_authenticated/tenants')({ beforeLoad: () => { throw redirect({ to: '/', search: { workspace: 'tenants' }, replace: true }); } });
