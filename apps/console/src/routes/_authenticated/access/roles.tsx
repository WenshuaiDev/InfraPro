import { createFileRoute } from '@tanstack/react-router';
import { RolesPage } from '../../../features/access/roles-page';
export const Route = createFileRoute('/_authenticated/access/roles')({ validateSearch:(s:Record<string,unknown>):{role?:string}=>({role:typeof s.role==='string'&&/^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/i.test(s.role)?s.role:undefined}), component: RolesPage });
