import { useEffect, useRef } from 'react';
import { useMutation, type UseMutationOptions } from '@tanstack/react-query';

// Share an in-flight write instead of queuing a duplicate, and discard UI callbacks
// after navigation/account changes. Writes are never automatically retried.
export function useOperation<TData = void, TVariables = void>(options: UseMutationOptions<TData, Error, TVariables>) {
  const mounted = useRef(true);
  const pending = useRef<Promise<TData> | undefined>(undefined);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  return useMutation({
    ...options, retry: false,
    mutationFn: (variables, context) => {
      if (!pending.current) {
        const action = options.mutationFn;
        if (!action) throw new Error('A mutation function is required');
        const request = Promise.resolve().then(() => action(variables, context));
        pending.current = request;
        void request.finally(() => { if (pending.current === request) pending.current = undefined; }).catch(() => undefined);
      }
      return pending.current;
    },
    onSuccess: (...args) => mounted.current ? options.onSuccess?.(...args) : undefined,
    onError: (...args) => mounted.current ? options.onError?.(...args) : undefined,
    onSettled: (...args) => mounted.current ? options.onSettled?.(...args) : undefined,
  });
}
