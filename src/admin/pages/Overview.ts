import { h, type Child } from '@/shared/core/Dom.ts';
import { section, table } from '@/shared/components/Ui.ts';
import { ago, bytes, when } from '@/shared/render/format.ts';
import { AdminPage } from '@/admin/pages/base.ts';

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
  const inner: Child[] = [h('span', { class: 'n' }, String(n)), h('span', { class: 'l' }, label)];
  return href === undefined ? h('div', { class: 'tile' }, ...inner) : h('a', { class: 'tile', href }, ...inner);
}

/** "Vue d'ensemble": counts, warnings, devices that stopped syncing. */
export class OverviewPage extends AdminPage {
  protected async render(): Promise<Child[]> {
    const o = await this.api.get<Overview>('/overview');
    return [
      ...o.warnings.map((w) => h('p', { class: 'banner error', 'data-code': w.code }, w.message)),
      h('div', { class: 'tiles' },
        tile(o.users, 'utilisateurs', '/admin/users'),
        tile(o.groups, 'groupes', '/admin/groups'),
        tile(o.calendars, 'calendriers', '/admin/calendars'),
        tile(o.events, 'événements'),
        tile(o.devices, 'appareils'),
      ),
      section('Serveur',
        h('p', {}, 'Accès public : ', o.public_url !== '' ? h('code', {}, o.public_url) : h('span', { class: 'muted' }, 'non configuré')),
        h('p', {}, `Base : ${bytes(o.storage.database)} · sauvegardes : ${bytes(o.storage.backups)}`),
        h('p', {}, 'Dernière sauvegarde : ', o.last_backup === null ? 'aucune' : `${when(o.last_backup)} (${ago(o.last_backup)})`),
        h('p', { class: 'muted' }, `Version ${o.version}`),
      ),
      section('Appareils sans nouvelles depuis une semaine',
        table(['Utilisateur', 'Appareil', 'Type', 'Vu'],
          o.stale_devices.map((d) => [h('a', { href: `/admin/users/${encodeURIComponent(d.user)}` }, d.user), d.label, d.kind, ago(d.last_used_at)]),
          'Tous les appareils se sont synchronisés cette semaine.'),
      ),
    ];
  }
}
