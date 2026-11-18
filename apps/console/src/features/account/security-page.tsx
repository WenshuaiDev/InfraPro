import { Permissions as P } from '@infrapro/api-client';
import { useIdentity } from '@infrapro/api-client/react';
import { AccountCenterLayout } from './account-center-layout';
import { MFACard } from './mfa-card';
import { PasswordCard } from './password-card';
import { SessionsCard } from './sessions-card';
import { SecurityRecordsCard } from './security-records';

export function SecurityPage() {
  const { can } = useIdentity();
  const sessions = can(P.AccountSessionRead) || can(P.AccountSessionRevoke);
  const records = can(P.AccountSecurityEventRead);
  const password = can(P.AccountPasswordUpdate);
  return <AccountCenterLayout title="账号安全">
    <div className={'grid gap-4 ' + (sessions || records || password ? 'md:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)]' : 'max-w-2xl')}>
      <MFACard />
      {sessions && <SessionsCard />}
      {records && <SecurityRecordsCard />}
      {password && <PasswordCard />}
    </div>
  </AccountCenterLayout>;
}
