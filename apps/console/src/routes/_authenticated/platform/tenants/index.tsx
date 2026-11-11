import { createFileRoute } from '@tanstack/react-router';
import { PlatformTenantsPage } from '../../../../features/tenancy/platform-tenants-page';
export const Route = createFileRoute('/_authenticated/platform/tenants/')({ component: PlatformTenantsPage });
