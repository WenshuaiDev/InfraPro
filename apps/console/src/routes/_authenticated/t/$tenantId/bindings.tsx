import { createFileRoute } from '@tanstack/react-router';
import { TenantDevices } from '../../../../features/devices/context';
import { BindingWorkspace } from '../../../../features/bindings/workspace';
import { bindingSearch } from '../../../../features/bindings/model';

export const Route = createFileRoute('/_authenticated/t/$tenantId/bindings')({
  validateSearch: bindingSearch,
  component: function Page() {
    const search = Route.useSearch();
    const navigate = Route.useNavigate();
    return <TenantDevices><BindingWorkspace search={search} change={next => void navigate({ search: next, replace: true, resetScroll: false })} /></TenantDevices>;
  },
});
