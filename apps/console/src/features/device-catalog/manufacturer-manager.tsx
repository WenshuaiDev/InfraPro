import { useState } from 'react';
import { Plus } from 'lucide-react';
import type { DeviceManufacturer } from '@infrapro/api-client';
import { Badge } from '@infrapro/ui/components/badge';
import { Button } from '@infrapro/ui/components/button';
import { DirectoryActions, DirectorySearch, DirectoryTable } from '@infrapro/ui/patterns/directory-table';
import { ListPagination } from '@infrapro/ui/patterns/list-controls';
import { useCursorList } from '../../hooks/use-cursor-list';
import { useDeviceCatalog } from './context';

export type ManufacturerMode = 'edit' | 'state' | 'delete';

export function ManufacturerManager({ onEdit, onSelect }: {
  onEdit: (manufacturer?: DeviceManufacturer, mode?: ManufacturerMode) => void;
  onSelect: (manufacturer: DeviceManufacturer) => void;
}) {
  const c = useDeviceCatalog();
  const [search, setSearch] = useState('');
  const [size, setSize] = useState(25);
  const manufacturers = useCursorList({
    queryKey: [...c.prefix, 'manufacturer-management', search, size],
    load: (cursor, signal) => c.client.deviceManufacturers(c.scope, { q: search, cursor, limit: size }, signal),
  });
  return <div className="space-y-5">
    <DirectoryTable label="厂家管理" items={manufacturers.items} rowId={(item) => item.id}
      loading={manufacturers.isPending} error={manufacturers.error?.message} retry={() => void manufacturers.refetch()}
      empty={search ? '没有符合搜索条件的厂家。' : '暂无厂家，可以先新建厂家。'}
      toolbar={<><DirectorySearch label="搜索厂家名称或编号" value={search} onChange={setSearch} /><Button id="create-manufacturer" disabled={!c.available} onClick={() => onEdit()}><Plus aria-hidden="true" />新建厂家</Button></>}
      columns={[
        { id: 'name', label: '厂家', cell: (item) => <div className="space-y-1"><p className="font-medium">{item.name}</p><p className="break-all text-xs text-muted-foreground">{item.code}</p></div> },
        { id: 'status', label: '状态', className: 'w-20', cell: (item) => <Badge variant={item.status === 'active' ? 'secondary' : 'outline'}>{item.status === 'active' ? '启用' : '停用'}</Badge> },
        { id: 'actions', label: '操作', className: 'w-14', cell: (item) => <DirectoryActions name={item.name} id={`manufacturer-actions-${item.id}`} actions={[
          { label: '查看产品', run: () => onSelect(item) },
          ...(['edit', 'state', 'delete'] as const).map((mode) => ({
            label: mode === 'edit' ? '编辑厂家' : mode === 'state' ? item.status === 'active' ? '停用厂家' : '启用厂家' : '删除厂家',
            disabled: !c.available, opensPanel: true, run: () => onEdit(item, mode),
          })),
        ]} /> },
      ]}
      pagination={<ListPagination label="厂家" count={manufacturers.items.length} page={manufacturers.page} pageCount={manufacturers.pageCount}
        hasNext={!!manufacturers.hasNext} fetching={manufacturers.isFetching} go={manufacturers.go} next={() => void manufacturers.next()}
        pageSize={size} setPageSize={setSize} pageSizes={[25, 50, 100]} />}
    />
    <p className="text-sm leading-relaxed text-muted-foreground">厂家停用后不能用于新建产品；既有产品的发布版本仍按各自状态管理。</p>
  </div>;
}
