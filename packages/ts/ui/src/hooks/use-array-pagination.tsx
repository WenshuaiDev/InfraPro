import { useState } from 'react';
import type { ListPage } from '../patterns/list-controls';

export function useArrayPagination<T>(items: readonly T[], initialSize = 20, scope = '') {
  const [position, setPosition] = useState({ scope, page: 0, pageSize: initialSize });
  const pageSize = position.pageSize;
  const page = Math.min(position.scope === scope ? position.page : 0, Math.max(0, Math.ceil(items.length / pageSize) - 1));
  if (position.scope !== scope || position.page !== page) setPosition({ scope, page, pageSize });
  const pagination: ListPage = {
    page, total: items.length, pageSize,
    go: page => setPosition({ scope, page, pageSize }),
    setPageSize: pageSize => setPosition({ scope, page: 0, pageSize }),
  };
  return { items: items.slice(page * pageSize, (page + 1) * pageSize), pagination };
}
