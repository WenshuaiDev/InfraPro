import { createRouter, defaultParseSearch } from '@tanstack/react-router';
import { routeTree } from '../routeTree.gen';
import type { RouterContext } from './router-context';

export function makeRouter(context: RouterContext) {
  return createRouter({ parseSearch: value => {
    const parsed=defaultParseSearch(value), execution=new URLSearchParams(value).get('execution');
    // Preserve plain decimal execution links before JSON parsing can round an
    // integer above Number.MAX_SAFE_INTEGER. Quoted router strings also work.
    if(execution && /^[1-9][0-9]*$/.test(execution)) return {...parsed,execution};
    return parsed;
  }, routeTree, context, basepath: import.meta.env.BASE_URL, defaultPreload: 'intent', defaultPendingMs: 150, scrollRestoration: true });
}

declare module '@tanstack/react-router' { interface Register { router: ReturnType<typeof makeRouter> } }
