import { createFileRoute } from '@tanstack/react-router';
import { ProjectProfilePage } from '../../../../../../features/projects/profile-page';
import { filesSearch } from '../../../../../../features/files/state';
export const Route = createFileRoute('/_authenticated/t/$tenantId/p/$projectId/profile')({
  validateSearch: filesSearch,
  component: function Page() {
    const search = Route.useSearch(), navigate = Route.useNavigate();
    return <ProjectProfilePage attachmentSearch={search} onAttachmentSearch={value => void navigate({ search: { ...value, tab: 'active' }, resetScroll: false })} />;
  },
});
