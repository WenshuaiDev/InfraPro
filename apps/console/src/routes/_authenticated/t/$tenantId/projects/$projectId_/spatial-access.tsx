import { createFileRoute } from "@tanstack/react-router";
import { TenantSpatial } from "../../../../../../features/spatial/context";
import { SpatialAccessPage } from "../../../../../../features/spatial/access-page";
export const Route = createFileRoute(
  "/_authenticated/t/$tenantId/projects/$projectId_/spatial-access",
)({
  validateSearch: (value: Record<string, unknown>): { history?: boolean } => ({
    history:
      value.history === true || value.history === "true" ? true : undefined,
  }),
  component: function Page() {
    const { projectId } = Route.useParams(),
      search = Route.useSearch(),
      navigate = Route.useNavigate();
    return (
      <TenantSpatial>
        <SpatialAccessPage
          project={projectId}
          showEnded={!!search.history}
          onShowEnded={(history) =>
            void navigate({
              search: { history: history || undefined },
              resetScroll: false,
            })
          }
        />
      </TenantSpatial>
    );
  },
});
