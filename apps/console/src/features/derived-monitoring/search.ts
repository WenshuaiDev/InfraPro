export function derivedSearch(value:Record<string,unknown>) {
 const text=(key:string)=>typeof value[key]==='string'?value[key] as string:undefined;
 return {evidence_export_id:text('evidence_export_id'),export_id:text('export_id'),id:text('id'),view:text('view'),version:text('version'),owner:text('owner'),owner_kind:text('owner_kind'),facility:text('facility')};
}
export type DerivedSearch=ReturnType<typeof derivedSearch>;
