import { createFileRoute } from '@tanstack/react-router';
import { PlatformTenantsPage } from '../../../../features/tenancy/pages';
export const Route = createFileRoute('/_authenticated/platform/tenants/')({ component: PlatformTenantsPage });
