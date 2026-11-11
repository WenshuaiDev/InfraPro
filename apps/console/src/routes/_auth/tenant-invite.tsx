import { createFileRoute } from '@tanstack/react-router';
import { TenantAcceptPage } from '../../features/tenancy/accept-page';
export const Route = createFileRoute('/_auth/tenant-invite')({ component: TenantAcceptPage });
