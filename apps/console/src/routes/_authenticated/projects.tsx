import { createFileRoute, redirect } from '@tanstack/react-router';
import { MyProjectsPage } from '../../features/projects/lists';
export const Route = createFileRoute('/_authenticated/projects')({ beforeLoad: ({ search }) => { if (import.meta.env.DEV && import.meta.env.VITE_NAVIGATION_PROTOTYPE === '1') throw redirect({ to: '/', search: { variant: search.variant ?? 'A', picker: 'project' }, replace: true }); }, component: function Page() { return <MyProjectsPage />; } });
