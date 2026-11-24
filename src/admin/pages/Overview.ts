import { ui } from '@/shared/components/recipes.ts';
import { icon } from '@/shared/components/icons.ts';
import { h, type Child } from '@/shared/core/Dom.ts';
import { section, table } from '@/shared/components/Ui.ts';
import { locale } from '@/shared/i18n/i18n.ts';
import { ago, bytes, when } from '@/shared/render/format.ts';
import { AdminPage } from '@/admin/pages/base.ts';
import { deviceKind } from '@/admin/pages/Users.ts';
import { t } from '@/admin/pages/Overview.msg.ts';

export interface Overview {
  users: number;
  groups: number;
  calendars: number;
  events: number;
  devices: number;
  stale_devices: { user: string; id: number; label: string; kind: string; last_used_at?: string }[];
  storage: { database: number; backups: number };
  last_backup: string | null;
  version: string;
  public_url: string;
  warnings: { code: string; message: string }[];
}

export function tile(n: number | string, label: string, href?: string): HTMLElement {
  const inner: Child[] = [h('span', { class: 'text-[32px] leading-10 font-semibold text-primary' }, typeof n === 'number' ? n.toLocaleString(locale()) : n), h('span', { class: 'text-sm text-on-surface-variant' }, label)];
  return href === undefined ? h('div', { class: 'state-layer flex flex-col gap-1 rounded-md3-xl bg-surface-container p-4 text-on-surface no-underline' }, ...inner) : h('a', { class: 'state-layer flex flex-col gap-1 rounded-md3-xl bg-surface-container p-4 text-on-surface no-underline', href }, ...inner);
}

/** "Vue d’ensemble": counts, warnings, devices that stopped syncing. */
/** The family's address, opening their side in a new tab (http and https only). */
function publicLink(url: string): HTMLElement {
  if (!/^https?:\/\//i.test(url)) return h('code', {}, url);
  return h('a', { href: url, target: '_blank', rel: 'noopener noreferrer', title: t('openPublic'),
    class: 'inline-flex items-center gap-1 font-mono text-primary underline-offset-4 hover:underline [&_.ic]:size-4' }, url, icon('link'));
}

export class OverviewPage extends AdminPage {
  protected async render(): Promise<Child[]> {
    const o = await this.api.get<Overview>('/overview');
    return [
      ...o.warnings.map((w) => h('p', { class: ui.error, 'data-code': w.code }, w.message)),
      h('div', { class: 'grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(150px,1fr))]' },
        tile(o.users, t('users', { count: o.users }), '/users'),
        tile(o.groups, t('groups', { count: o.groups }), '/groups'),
        tile(o.calendars, t('calendars', { count: o.calendars }), '/calendars'),
        tile(o.events, t('events', { count: o.events })),
        tile(o.devices, t('devices', { count: o.devices })),
      ),
      section(t('server'),
        h('p', {}, t('publicUrl'), ' ', o.public_url !== '' ? publicLink(o.public_url) : h('span', { class: 'text-sm text-on-surface-variant' }, t('notConfigured'))),
        h('p', {}, t('storage', { database: bytes(o.storage.database), backups: bytes(o.storage.backups) })),
        h('p', {}, o.last_backup === null ? t('noBackup') : t('lastBackup', { when: when(o.last_backup), ago: ago(o.last_backup) })),
        h('p', { class: 'text-sm text-on-surface-variant' }, t('version', { version: o.version })),
      ),
      section(t('staleTitle'),
        table([t('user'), t('device'), t('kind'), t('lastSync')],
          o.stale_devices.map((d) => [h('a', { href: `/users/${encodeURIComponent(d.user)}` }, d.user), d.label, deviceKind(d.kind), ago(d.last_used_at)]),
          t('staleEmpty')),
      ),
    ];
  }
}
