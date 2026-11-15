import { createFileRoute } from '@tanstack/react-router';
import { TaskCenter } from '../../features/tasks/center';
import { taskSearch } from '../../features/tasks/search';
export const Route = createFileRoute('/_authenticated/tasks')({ validateSearch: taskSearch, component: function Page() { return <TaskCenter filter={Route.useSearch()} />; } });
