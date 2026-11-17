export type GovernanceView = 'summary' | 'access';
export type GovernanceSearch = {
  q?: string;
  status?: 'all' | 'active' | 'archived';
  owner?: string;
  unavailable?: boolean;
  project?: string;
  view?: GovernanceView;
};

const id = (value: unknown) => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value) ? value : undefined;

export function readGovernanceSearch(raw: Record<string, unknown>): GovernanceSearch {
  return {
    q: typeof raw.q === 'string' ? Array.from(raw.q).slice(0, 100).join('') || undefined : undefined,
    status: raw.status === 'active' || raw.status === 'archived' ? raw.status : 'all',
    owner: id(raw.owner),
    unavailable: raw.unavailable === true || raw.unavailable === 'true' ? true : undefined,
    project: id(raw.project),
    view: id(raw.project) && raw.view === 'access' ? 'access' : undefined,
  };
}

export function plannedEndNotice(end: string, status: string, now = new Date()) {
  if (!end || status !== 'active') return '';
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(now);
  const get = (type: string) => parts.find(part => part.type === type)?.value;
  const today = Date.UTC(Number(get('year')), Number(get('month')) - 1, Number(get('day')));
  const days = Math.round((Date.parse(end + 'T00:00:00Z') - today) / 86_400_000);
  if (days < 0) return '计划结束日已过';
  if (days === 0) return '今天计划到期';
  return days <= 30 ? days + ' 天后计划到期' : '';
}

export const projectDate = (value: string) => value ? value.replaceAll('-', '.') : '未设置';
