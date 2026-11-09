import { createFileRoute } from '@tanstack/react-router';
import { PermissionsPage } from '../../../features/access/permissions-page';
export const Route = createFileRoute('/_authenticated/access/permissions')({ component: PermissionsPage });
