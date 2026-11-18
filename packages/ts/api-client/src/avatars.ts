export type AvatarTarget =
  | { kind: 'self' }
  | { kind: 'platform'; id: string }
  | { kind: 'tenant'; tenant: string; id: string }
  | { kind: 'project'; tenant: string; project: string; id: string };

export function avatarPath(target: AvatarTarget): string {
  const id = (value: string) => encodeURIComponent(value);
  switch (target.kind) {
    case 'self': return '/account/avatar';
    case 'platform': return `/access/accounts/${id(target.id)}/avatar`;
    case 'tenant': return `/tenants/${id(target.tenant)}/members/${id(target.id)}/avatar`;
    case 'project': return `/tenants/${id(target.tenant)}/projects/${id(target.project)}/members/${id(target.id)}/avatar`;
  }
}
