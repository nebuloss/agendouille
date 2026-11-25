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
  /** The device's id on the server (0 when signed in with a device's password). */
  device_id: number;
}

export interface CalendarState {
  /** The phone's calendar id. */
  id: number;
  name: string;
  color: string;
  sync: boolean;
  read_only: boolean;
  /** The server's reference ("lea~famille"), as the API names it. */
  ref: string;
}

export interface SyncState {
  state: 'never' | 'idle' | 'syncing' | 'error';
  last_sync: string | null;
  error: string | null;
  events: number;
}

export interface AppState {
  account: AccountState | null;
  permissions: {
    calendar: boolean;
    camera: boolean;
    /** Notifications (Android 13 on asks): sync errors and updates. */
    notifications: boolean;
    /** Left out of battery optimisation: background syncs not held back. */
    battery: boolean;
  };
  /** A Xiaomi phone (MIUI): background starts need « Démarrage automatique » too. */
  xiaomi: boolean;
  /** Updates from the server's dev build (on) or GitHub's releases only (off). */
  devUpdates: boolean;
  /** An update on offer (Java's check), for the dialog; null when none. */
  update: UpdateOffer | null;
  /** Instant sync (UnifiedPush): the distributors installed, the one in use, whether it works. */
  push: PushState;
  sync: SyncState;
  calendars: CalendarState[];
  /** What made the app stop last time, or null. */
  crash: string | null;
}

export const EMPTY: AppState = {
  account: null,
  permissions: { calendar: false, camera: false, notifications: false, battery: false },
  xiaomi: false,
  devUpdates: false,
  update: null,
  push: { instant: false, modeChosen: false, service: false, connected: false, hidden: false, users: [] },
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
      ? { server: str(a['server']), host: str(a['host']), username: str(a['username']), mode: a['mode'] === 'manual' ? 'manual' : a['mode'] === 'qr' ? 'qr' : 'enrolled',
        device_id: typeof a['device_id'] === 'number' ? a['device_id'] : 0 }
      : null,
    permissions: { calendar: p['calendar'] === true, camera: p['camera'] === true, notifications: p['notifications'] === true, battery: p['battery'] === true },
    xiaomi: o['xiaomi'] === true,
    devUpdates: o['dev_updates'] === true,
    push: parsePush(o['push']),
    update: parseUpdate(o['update']),
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
      ref: str(c['ref']),
    })),
  };
}

/** Instant sync as Java reports it (MainActivity.pushInfo). */
export interface PushState {
  /** The person turned it on (Réglages). */
  instant: boolean;
  /** The person chose a mode (the first screen after signing in). */
  modeChosen: boolean;
  /** The server has its push service (/api/app/config). */
  service: boolean;
  /** The stream is open now. */
  connected: boolean;
  /** Its notification's category is turned off in Android's settings. */
  hidden: boolean;
  /** The apps that use this one as their UnifiedPush distributor (DAVx5). */
  users: string[];
}

function parsePush(v: unknown): PushState {
  const o = (v ?? {}) as Record<string, unknown>;
  return {
    instant: o['instant'] === true,
    modeChosen: o['mode_chosen'] === true,
    service: o['service'] === true,
    connected: o['connected'] === true,
    hidden: o['hidden'] === true,
    users: Array.isArray(o['users']) ? (o['users'] as unknown[]).map(String) : [],
  };
}

/** An update on offer: its version, this one, and where it comes from. */
export interface UpdateOffer {
  version: string;
  current: string;
  source: 'server' | 'github';
  /** Offered again each time: the dialog shows once per offer. */
  at: number;
}

function parseUpdate(v: unknown): UpdateOffer | null {
  if (v === null || typeof v !== 'object') return null;
  const o = v as Record<string, unknown>;
  if (typeof o['version'] !== 'string') return null;
  return { version: o['version'], current: String(o['current'] ?? ''), source: o['source'] === 'github' ? 'github' : 'server', at: Number(o['at'] ?? 0) };
}
