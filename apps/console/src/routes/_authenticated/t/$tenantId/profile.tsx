import { createFileRoute } from '@tanstack/react-router';
import { TenantProfilePage } from '../../../../features/tenancy/profile-page';
import { filesSearch } from '../../../../features/files/state';

export const Route = createFileRoute('/_authenticated/t/$tenantId/profile')({
  validateSearch: filesSearch,
  component: function ProfilePage() {
    const search = Route.useSearch(), navigate = Route.useNavigate();
    return <TenantProfilePage attachmentSearch={search} onAttachmentSearch={value => void navigate({ search: { ...value, tab: 'active' }, resetScroll: false })} />;
  },
});
