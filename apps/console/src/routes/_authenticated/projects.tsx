import { createFileRoute } from '@tanstack/react-router';
import { MyProjectsPage } from '../../features/projects/lists';
export const Route = createFileRoute('/_authenticated/projects')({ component: function Page() { return <MyProjectsPage />; } });
