import { useState } from 'react';
import type { DeviceClassificationSelection } from '@infrapro/api-client';
import { Button } from '@infrapro/ui/components/button';
import { ResponsivePanel } from '@infrapro/ui/patterns/responsive-panel';
import { ListFilter } from '@infrapro/ui/patterns/list-controls';
import { CheckControl } from '../engineering-fields/controls';
import { CategoryPicker } from '../classification/picker';
import { useDevices } from './context';
import type { TenantDeviceSearch } from './search';
export function ClassificationFilter({search,onChange}:{search:TenantDeviceSearch;onChange:(s:TenantDeviceSearch)=>void}){
 const {scope}=useDevices();const [open,setOpen]=useState(false);const [selection,setSelection]=useState<DeviceClassificationSelection>({mode:'classify',category_id:search.category});const [descendants,setDescendants]=useState(search.category_scope==='descendants');
 return <><ListFilter label="设备分类组" value={search.category_group??''} onChange={v=>onChange({...search,category_group:(v||undefined) as TenantDeviceSearch['category_group']})} options={[{value:'',label:'全部分类组'},{value:'sensor',label:'传感器'},{value:'non_sensor',label:'非传感器'},{value:'unclassified',label:'未分类'},{value:'unavailable',label:'分类解释不可用'}]}/><Button id="category-filter" variant="outline" className="h-9 border-dashed max-sm:h-11" onClick={()=>setOpen(true)}>{search.category?'已指定设备分类':'按设备分类筛选'}</Button><ResponsivePanel open={open} onOpenChange={setOpen} title="筛选设备分类" description="可选择分类本身，或包含该分类的全部后代。" fallbackFocusSelector="#category-filter"><div className="space-y-4"><CategoryPicker scope={scope} value={selection} onChange={setSelection} filterOnly/><CheckControl label="包含所有后代分类" value={descendants} onChange={setDescendants}/><div className="flex gap-2"><Button disabled={selection.mode==='classify'&&!selection.category_id} onClick={()=>{onChange({...search,category:selection.mode==='classify'?selection.category_id:undefined,category_scope:descendants?'descendants':'exact'});setOpen(false);}}>应用筛选</Button><Button variant="outline" onClick={()=>{onChange({...search,category:undefined,category_scope:undefined});setOpen(false);}}>清除分类筛选</Button></div></div></ResponsivePanel></>;
}
