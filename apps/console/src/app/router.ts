import { createRouter } from '@tanstack/react-router';
import { routeTree } from '../routeTree.gen';
import type { RouterContext } from './router-context';

export function makeRouter(context: RouterContext) {
  return createRouter({ routeTree, context, basepath: import.meta.env.BASE_URL, defaultPreload: 'intent', defaultPendingMs: 150, scrollRestoration: true });
}

declare module '@tanstack/react-router' { interface Register { router: ReturnType<typeof makeRouter> } }
