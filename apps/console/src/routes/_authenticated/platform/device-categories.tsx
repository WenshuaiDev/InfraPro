import { createFileRoute } from '@tanstack/react-router';
import { DeviceCategoryPage } from '../../../features/classification/dictionary';
export const Route = createFileRoute('/_authenticated/platform/device-categories')({component:DeviceCategoryPage});
