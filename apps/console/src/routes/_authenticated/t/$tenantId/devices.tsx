import { createFileRoute, useParams } from '@tanstack/react-router';
import { TenantDevices } from '../../../../features/devices/context';
import { TenantDeviceWorkspace } from '../../../../features/devices/workspace';
import { tenantDeviceSearch } from '../../../../features/devices/search';

export const Route = createFileRoute('/_authenticated/t/$tenantId/devices')({
  validateSearch: tenantDeviceSearch,
  component: function Page() {
    const { deviceId } = useParams({ strict: false });
    const search = Route.useSearch();
    const navigate = Route.useNavigate();
    return <TenantDevices><TenantDeviceWorkspace id={deviceId} search={search}
      onSelect={(id, next) => {
        if (id) void navigate({ to: '/t/$tenantId/devices/$deviceId', params: { deviceId: id }, search: next, resetScroll: false });
        else void navigate({ to: '/t/$tenantId/devices', search: next, resetScroll: false });
      }}
      onFilter={(next) => void navigate({ to: '/t/$tenantId/devices', search: next, resetScroll: false, replace: true })}
      onDetail={(next) => {
        if (deviceId) void navigate({ to: '/t/$tenantId/devices/$deviceId', params: { deviceId }, search: next, resetScroll: false, replace: true });
        else void navigate({ to: '/t/$tenantId/devices', search: next, resetScroll: false, replace: true });
      }} /></TenantDevices>;
  },
});
