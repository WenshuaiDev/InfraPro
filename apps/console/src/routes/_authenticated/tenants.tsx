import { createFileRoute } from '@tanstack/react-router';
import { MyTenantsPage } from '../../features/tenancy/pages';
export const Route = createFileRoute('/_authenticated/tenants')({ component: MyTenantsPage });
