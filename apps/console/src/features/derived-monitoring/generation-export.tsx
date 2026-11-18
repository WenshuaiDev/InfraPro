import { useState } from 'react';
import { Button } from '@infrapro/ui/components/button';
import type { DerivedGenerationQuery } from '@infrapro/api-client';
import { useOperation } from '../account/use-operation';
import { useRequestMeta } from '../engineering/use-request-meta';
import { AnalysisExportTask } from './export-tasks';
import { CommandError, Section, SelectField, TextField, useDerived } from './common';

export function GenerationEvidenceExport({generation,range}:{generation:string;range:NonNullable<DerivedGenerationQuery['output_range']>}) {
 const m=useDerived(),meta=useRequestMeta(),[format,setFormat]=useState<'csv'|'parquet'>('csv'),[mode,setMode]=useState<'exact'|'spreadsheet_safe'>('exact'),[reason,setReason]=useState(''),[id,setID]=useState('');
 const request=useOperation({mutationFn:()=>{const body={output_range:range,format,csv_mode:format==='parquet'?'exact' as const:mode};return m.client.generationCreateExport(m.scope,generation,{...body,...meta('generation-evidence-export',body,reason)});},onSuccess:v=>setID(v.id)});
 return <Section title="导出此范围的完整输入依据"><p className="text-sm text-muted-foreground">按所选历史代次读取原固定候选输入、上游结果和基准。需要结果及全部来源时段的历史与导出权限，包含已经撤销的原依据。清单说明规则、质量排除条件与身份，文件到期后可以重新申请。</p><div className="grid gap-3 sm:grid-cols-2"><SelectField label="输入依据文件格式" value={format} onChange={setFormat} options={[{value:'csv',label:'CSV 精确文本'},{value:'parquet',label:'Parquet 精确文本列'}]}/>{format==='csv'&&<SelectField label="输入依据 CSV 模式" value={mode} onChange={setMode} options={[{value:'exact',label:'精确原文，适合程序处理'},{value:'spreadsheet_safe',label:'表格安全模式'}]}/>}</div><TextField label="输入依据导出原因" value={reason} onChange={setReason}/><CommandError error={request.error}/><Button disabled={request.isPending} onClick={()=>request.mutate()}>后台生成完整输入依据</Button>{id&&<><AnalysisExportTask id={id} generation/></>}</Section>;
}
