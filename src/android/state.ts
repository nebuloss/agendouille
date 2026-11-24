/**
 * What the Android app tells its screens (CONTRACTS.md 6, version 2): the
 * account, the permissions, the sync and the calendars on the phone. The
 * Java side owns all of it; the page only shows it and asks for changes.
 */

export interface AccountState {
  server: string;
  host: string;
  username: string;
  /** How the phone was signed in: an enrollment link, or by hand. */
  mode: 'enrolled' | 'manual' | 'qr';
}

export interface CalendarState {
  /** The phone's calendar id. */
  id: number;
  name: string;
  color: string;
  sync: boolean;
  read_only: boolean;
}

export interface SyncState {
  state: 'never' | 'idle' | 'syncing' | 'error';
  last_sync: string | null;
  error: string | null;
  events: number;
}

export interface AppState {
  account: AccountState | null;
  permissions: { calendar: boolean; camera: boolean };
  sync: SyncState;
  calendars: CalendarState[];
  /** What made the app stop last time, or null. */
  crash: string | null;
}

export const EMPTY: AppState = {
  account: null,
  permissions: { calendar: false, camera: false },
  sync: { state: 'never', last_sync: null, error: null, events: 0 },
  calendars: [],
  crash: null,
};

/** Reads what the app sent, keeping only what the screens use. */
export function parseState(v: unknown): AppState {
  if (v === null || typeof v !== 'object') return EMPTY;
  const o = v as Record<string, unknown>;
  const a = o['account'] as Record<string, unknown> | null | undefined;
  const p = (o['permissions'] ?? {}) as Record<string, unknown>;
  const s = (o['sync'] ?? {}) as Record<string, unknown>;
  const cals = Array.isArray(o['calendars']) ? (o['calendars'] as Record<string, unknown>[]) : [];
  const str = (x: unknown): string => (typeof x === 'string' ? x : '');
  return {
    account: a !== null && a !== undefined && typeof a['server'] === 'string'
      ? { server: str(a['server']), host: str(a['host']), username: str(a['username']), mode: a['mode'] === 'manual' ? 'manual' : a['mode'] === 'qr' ? 'qr' : 'enrolled' }
      : null,
    permissions: { calendar: p['calendar'] === true, camera: p['camera'] === true },
    sync: {
      state: (['never', 'idle', 'syncing', 'error'] as const).find((x) => x === s['state']) ?? 'never',
      last_sync: typeof s['last_sync'] === 'string' ? s['last_sync'] : null,
      error: typeof s['error'] === 'string' ? s['error'] : null,
      events: typeof s['events'] === 'number' ? s['events'] : 0,
    },
    crash: typeof o['crash'] === 'string' && o['crash'] !== '' ? o['crash'] : null,
    calendars: cals.map((c) => ({
      id: typeof c['id'] === 'number' ? c['id'] : 0,
      name: str(c['name']),
      color: str(c['color']),
      sync: c['sync'] !== false,
      read_only: c['read_only'] === true,
    })),
  };
}
