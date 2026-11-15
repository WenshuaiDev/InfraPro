export type DerivedSearch={evidence_export_id?:string;export_id?:string;id?:string;view?:string;version?:string;owner?:string;owner_kind?:string;facility?:string;channel?:string;from?:string;to?:string};
export function derivedSearch(value:Record<string,unknown>):DerivedSearch {
 const text=(key:string)=>typeof value[key]==='string'?value[key] as string:undefined;
 return {channel:text('channel'),from:text('from'),to:text('to'),evidence_export_id:text('evidence_export_id'),export_id:text('export_id'),id:text('id'),view:text('view'),version:text('version'),owner:text('owner'),owner_kind:text('owner_kind'),facility:text('facility')};
}
