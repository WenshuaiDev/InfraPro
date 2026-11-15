import { useQuery } from '@tanstack/react-query';
import { Link } from '@tanstack/react-router';
import { AlertTriangle, ArrowUpRight, Building2, History, Pencil } from 'lucide-react';
import { ApiError, Permissions as P, type TenantProfile } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { Button } from '@infrapro/ui/components/button';
import { ErrorState, FormNotice, LoadingState } from '@infrapro/ui/patterns/feedback';
import { WorkspacePickerButton } from '../../app/workspace-picker-context';
import { ObjectHistory } from '../audit/object-history';
import { AttachmentPanel } from '../files/attachment-panel';
import { TenantFiles } from '../files/context';
import { fileDate } from '../files/file-format';
import type { FileSearch } from '../files/state';
import { useTenant } from './context';
import { TenantEditor } from './editor';

export function TenantProfilePage({ attachmentSearch, onAttachmentSearch }: {
  attachmentSearch: FileSearch;
  onAttachmentSearch: (search: FileSearch) => void;
}) {
  const ctx = useTenant(), { account, client } = useIdentity();
  const readable = ctx.permissions.includes(P.TenantProfileRead);
  const profile = useQuery({
    queryKey: ['tenant', account?.id, ctx.tenant.id, ctx.membership_id, 'profile'],
    queryFn: ({ signal }) => client.tenant(ctx.tenant.id, false, signal),
    enabled: ctx.available && readable,
    refetchInterval: 30_000,
    retry: false,
  });
  const accessLost = profile.error instanceof ApiError && [401, 403, 404].includes(profile.error.status ?? 0);
  return <div className="mx-auto w-full max-w-6xl space-y-6">
    <h1 className="text-sm font-medium text-muted-foreground">租户资料</h1>
    {!readable ? <div className="space-y-4"><FormNotice>当前未获资料查看权限，请联系租户管理员。</FormNotice><WorkspacePickerButton filter="tenants">重新选择租户</WorkspacePickerButton></div> : <>
      {profile.error && <ErrorState message={profile.error.message} onRetry={() => void profile.refetch()} />}
      {!accessLost && (profile.data ? <TenantProfileCard
        profile={profile.data}
        blocked={!ctx.available || !!profile.error}
        retry={() => void profile.refetch()}
        attachmentSearch={attachmentSearch}
        onAttachmentSearch={onAttachmentSearch}
      /> : !profile.error && <LoadingState label="正在读取租户资料…" />)}
    </>}
  </div>;
}

function TenantProfileCard({ profile, blocked, retry, attachmentSearch, onAttachmentSearch }: {
  profile: TenantProfile;
  blocked: boolean;
  retry: () => void;
  attachmentSearch: FileSearch;
  onAttachmentSearch: (search: FileSearch) => void;
}) {
  const ctx = useTenant();
  const editable = ctx.permissions.includes(P.TenantProfileUpdate);
  const administrator = profile.administrator;
  return <>
    <header className="flex flex-wrap items-start gap-4 border-b pb-7 sm:gap-6 sm:pb-8">
      <span className="flex size-14 shrink-0 items-center justify-center rounded-2xl bg-emerald-50 text-emerald-800 sm:size-[72px] dark:bg-emerald-950/50 dark:text-emerald-300"><Building2 aria-hidden="true" className="size-7 stroke-[1.5] sm:size-9" /></span>
      <div className="min-w-0 flex-1 basis-48">
        <h2 className="break-words text-2xl leading-snug font-semibold tracking-tight sm:text-3xl">{profile.name}</h2>
        <p className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground"><span>租户工作空间</span><span aria-hidden="true">·</span><span className="break-all font-mono" aria-label={`租户编号 ${profile.code}`}>{profile.code}</span>{!editable && <span className="rounded border px-1.5 py-0.5">仅查看</span>}</p>
      </div>
      {editable && <TenantEditor mode="edit" tenant={profile} platform={false} blocked={blocked} retry={retry} trigger={<Button variant="outline" className="ml-auto"><Pencil aria-hidden="true" />编辑资料</Button>} />}
    </header>
    {!profile.administrator_available && <div role="status" className="flex gap-3 rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-950 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-100"><AlertTriangle aria-hidden="true" className="mt-0.5 size-5 shrink-0" /><div><p className="font-medium">租户管理员暂不可用</p><p className="mt-1 leading-6">请联系平台管理员核查账号状态，恢复管理员可用性或办理交接。</p></div></div>}
    <div className="grid min-w-0 gap-8 lg:grid-cols-[minmax(0,1fr)_15rem] lg:gap-10">
      <div className="min-w-0">
        <section aria-labelledby="tenant-about-title" className="pb-7 sm:pb-8">
          <h2 id="tenant-about-title" className="text-base font-semibold">关于本租户</h2>
          <p className={`mt-3 whitespace-pre-wrap break-words text-sm leading-7${profile.description ? '' : ' text-muted-foreground'}`}>{profile.description || '尚未填写租户说明。'}</p>
        </section>
        <div className="border-t pt-5 sm:pt-6">
          <TenantFiles><AttachmentPanel layout="list" search={attachmentSearch} onSearch={onAttachmentSearch} /></TenantFiles>
        </div>
      </div>
      <aside aria-label="租户管理信息" className="grid min-w-0 gap-6 border-t pt-6 sm:grid-cols-2 lg:block lg:space-y-7 lg:border-t-0 lg:border-l lg:pt-0 lg:pl-7">
        <section aria-labelledby="tenant-administrator-title" className="min-w-0 lg:border-b lg:pb-6">
          <h2 id="tenant-administrator-title" className="text-xs font-medium text-muted-foreground">管理负责人</h2>
          <div className="mt-4 flex items-start gap-3"><span aria-hidden="true" className="flex size-9 shrink-0 items-center justify-center rounded-full border bg-muted/50 text-xs font-medium">{Array.from(administrator.display_name || administrator.username)[0]}</span><div className="min-w-0"><p className="break-words text-sm font-medium">{administrator.display_name}</p><p className="mt-0.5 break-all text-xs text-muted-foreground">{administrator.username}</p></div></div>
          <p className="mt-3 text-xs leading-6 text-muted-foreground">成员加入、项目管理等事项，可联系租户管理员。</p>
          {ctx.permissions.includes(P.TenantMemberRead) && <Button variant="ghost" size="sm" className="mt-2 -ml-2 text-emerald-800 dark:text-emerald-300" asChild><Link to="/t/$tenantId/members" params={{ tenantId: profile.id }}>查看成员<ArrowUpRight aria-hidden="true" /></Link></Button>}
        </section>
        <section aria-labelledby="tenant-maintenance-title" className="min-w-0">
          <h2 id="tenant-maintenance-title" className="text-xs font-medium text-muted-foreground">资料维护</h2>
          <dl className="mt-4 text-xs"><dt className="text-muted-foreground">最近更新</dt><dd className="mt-1.5 tabular-nums"><time dateTime={profile.updated_at}>{fileDate(profile.updated_at, true)}</time></dd></dl>
          <p className="mt-1 text-xs text-muted-foreground">上海时间</p>
          <ObjectHistory objectType="tenant" objectId={profile.id} variant="ghost" className="mt-2 -ml-3 text-xs text-muted-foreground" label={<><History aria-hidden="true" />变更记录</>} />
        </section>
      </aside>
    </div>
  </>;
}
