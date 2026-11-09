import { createRootRouteWithContext, Outlet } from '@tanstack/react-router';
import type { RouterContext } from '../app/router-context';
import { NotFoundPage, RouteErrorPage } from '../app/error-pages';

export const Route = createRootRouteWithContext<RouterContext>()({
  component: Outlet, notFoundComponent: NotFoundPage, errorComponent: RouteErrorPage,
});
