export type DomainFileScope={kind:'tenant';tenantId:string}|{kind:'project';tenantId:string;projectId:string};
export type ManagedFileRef={kind:'managed';scope:DomainFileScope;attachmentId:string;revisionId:string};
export type DomainFileRef=
 |{kind:'audit.export';scope:DomainFileScope|{kind:'platform'};exportId:string;generation:string}
 |{kind:'monitoring.import.source';scope:DomainFileScope;batchId:string}
 |{kind:'monitoring.export'|'derived.analysis.export'|'derived.evidence.export';scope:DomainFileScope;exportId:string;part:string}
 |{kind:'spatial.source';scope:DomainFileScope;versionId:string}
 |{kind:'spatial.file';scope:DomainFileScope;versionId:string;fileId:string}
 |{kind:'display.package';scope:Extract<DomainFileScope,{kind:'project'}>;instanceId:string;packageId:string};
export type FileDescriptor={ref:ManagedFileRef|DomainFileRef;display_name:string;media_type:string;size_bytes?:string;sha256?:string;state:'available'|'preparing'|'expired'|'invalidated'|'missing'|'restricted'|'unknown';expires_at?:string;preview_state?:string;allowed_actions:('download'|'preview')[]};
