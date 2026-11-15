import { createContext, useContext, useEffect, type ReactNode } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useIdentity } from '@infrapro/api-client/react';
import { Permissions as P, type FileScope } from '@infrapro/api-client';
import { FileState } from '@infrapro/ui/patterns/files';
import { useTenant } from '../tenancy/context';
import { useProject } from '../projects/context';
const Context=createContext<{scope:FileScope;prefix:readonly unknown[]}|null>(null);
export function TenantFiles({children}:{children:ReactNode}) {const t=useTenant(),{account,authorization_revision}=useIdentity();return <Boundary scope={{tenant:t.tenant.id}} prefix={['tenant',account?.id,t.tenant.id,t.membership_id,'files',authorization_revision,t.permissions.join(',')]} readable={t.permissions.includes(P.TenantProfileRead)&&t.permissions.includes(P.TenantAttachmentsRead)}>{children}</Boundary>;}
export function ProjectFiles({children}:{children:ReactNode}) {const p=useProject(),{authorization_revision}=useIdentity();return <Boundary scope={{tenant:p.project.tenant_id,project:p.project.id}} prefix={[...p.key,'files',authorization_revision,p.authorization.permissions.join(',')]} readable={p.can(P.ProjectProfileRead)&&p.can(P.ProjectAttachmentsRead)}>{children}</Boundary>;}
function Boundary({scope,prefix,readable,children}:{scope:FileScope;prefix:readonly unknown[];readable:boolean;children:ReactNode}) {const cache=useQueryClient(),stamp=JSON.stringify(prefix);useEffect(()=>{const key=JSON.parse(stamp) as unknown[];return ()=>{void cache.cancelQueries({queryKey:key});cache.removeQueries({queryKey:key});};},[cache,stamp]);return readable?<Context value={{scope,prefix}}><div key={stamp}>{children}</div></Context>:<FileState>当前没有本空间普通附件查看权限；资料查看权与附件权限分别授予。</FileState>;}
// eslint-disable-next-line react-refresh/only-export-components
export function useFiles() {const value=useContext(Context);if(!value)throw Error('Files boundary missing');const {client}=useIdentity(),cache=useQueryClient();const capabilities=useQuery({queryKey:[...value.prefix,'capabilities'],queryFn:({signal})=>client.fileCapabilities(value.scope,signal),refetchInterval:30000,refetchIntervalInBackground:false,retry:false});return {...value,client,capabilities,changed:async()=>{await cache.invalidateQueries({queryKey:value.prefix});await cache.invalidateQueries({queryKey:['tasks']});},path:`/t/${value.scope.tenant}${value.scope.project?`/p/${value.scope.project}`:''}/files`};}
