import { createFileRoute, Navigate } from '@tanstack/react-router';

// Public definitions are chosen where they are used; old bookmarks lead there.
export const Route = createFileRoute('/_authenticated/t/$tenantId/engineering/catalog/$kind')({
  component: function Page() {
    const { tenantId, kind } = Route.useParams();
    return kind === 'monitoring-templates'
      ? <Navigate to="/t/$tenantId/engineering/templates" params={{ tenantId }} replace />
      : <Navigate to="/t/$tenantId/resources/facilities" params={{ tenantId }} replace />;
  },
});
