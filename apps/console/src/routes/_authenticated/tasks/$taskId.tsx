import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { TaskDetailContent } from '../../../features/tasks/detail';
export const Route = createFileRoute('/_authenticated/tasks/$taskId')({ component: function Page() {
 const { taskId } = Route.useParams(), search = Route.useSearch(), navigate = useNavigate();
 return <ResponsivePanel open onOpenChange={open => { if (!open) void navigate({ to: '/tasks', search: { ...search, execution: undefined, action: undefined }, state: state => ({ ...state }) }); }} title="任务详情" description="查看固定执行代次、真实进度与当前可用结果。" fallbackFocusSelector={`#task-${taskId}`} ><TaskDetailContent key={taskId} id={taskId} execution={search.execution} initialAction={search.action} onExecution={execution => { void navigate({ to: '/tasks/$taskId', params: { taskId }, search: { ...search, execution, action: undefined }, state: state => ({ ...state }) }); }} /></ResponsivePanel>;
} });
