import { createFileRoute } from '@tanstack/react-router';

// The parent renders the selected execution inline or in a mobile sheet.
export const Route = createFileRoute('/_authenticated/tasks/$taskId')({});
