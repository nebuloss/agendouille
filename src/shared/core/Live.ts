/**
 * The topics the server announces changes of (internal/live), in its order,
 * and the tab's one stream of notices: claudication's (web/src/api/live.ts),
 * in plain TypeScript. internal/live's TestTopicsMirrored holds this list to
 * the server's.
 *
 * A notice carries no data: a page told reads again, through the API. The
 * stream is open on the topics the mounted pages name, closed while the tab
 * is hidden (which reads every page once when it is shown), and every page
 * reads once when a dropped stream comes back, since what changed while it
 * was down was told to nobody.
 */
export const TOPICS = [
  'people',
  'devices',
  'calendars',
  'activity',
  'settings',
  'notices',
  'backups',
] as const;

export type Topic = (typeof TOPICS)[number];

type Listener = () => void;

/** How long a change to the topics wanted waits for others before the stream is opened again. */
const SETTLE_MS = 50;
/** How long after the browser gave up on a stream it is opened again. */
const RETRY_MS = 5_000;
/** How long notices are gathered before pages are told: one write can change two topics. */
const GATHER_MS = 30;
/** While the stream is down, pages still read this often, as a floor. */
const FLOOR_MS = 30_000;

export class LiveStream {
  private source: EventSource | null = null;
  private opened = '';
  private readonly wanted = new Map<Topic, Set<Listener>>();
  private settle = 0;
  private retry = 0;
  private gather = 0;
  private floor = 0;
  private readonly due = new Set<Listener>();
  private connected = false;
  private dropped = false;
  private visible: boolean;

  constructor(private readonly win: Window, private readonly url: string) {
    this.visible = win.document.visibilityState === 'visible';
    win.document.addEventListener('visibilitychange', () => {
      const visible = win.document.visibilityState === 'visible';
      if (visible === this.visible) return;
      this.visible = visible;
      if (visible) this.fireAll();
      this.schedule();
    });
  }

  /** Has `fn` called when any of `topics` changes; returns what stops it. */
  on(topics: readonly Topic[], fn: Listener): () => void {
    for (const t of topics) {
      let set = this.wanted.get(t);
      if (set === undefined) this.wanted.set(t, (set = new Set()));
      set.add(fn);
    }
    this.schedule();
    return () => {
      for (const t of topics) {
        const set = this.wanted.get(t);
        set?.delete(fn);
        if (set?.size === 0) this.wanted.delete(t);
      }
      this.schedule();
    };
  }

  /** Whether notices are arriving. */
  get live(): boolean {
    return this.connected;
  }

  private schedule(): void {
    this.win.clearTimeout(this.settle);
    this.settle = this.win.setTimeout(() => this.open(), SETTLE_MS);
  }

  private topics(): string {
    return TOPICS.filter((t) => this.wanted.has(t)).join(',');
  }

  private open(): void {
    const topics = this.visible ? this.topics() : '';
    if (topics === this.opened && this.source !== null) return;
    this.close();
    if (topics === '') return;
    this.opened = topics;
    const source = new EventSource(`${this.url}?topics=${topics}`);
    this.source = source;
    source.onopen = () => {
      if (this.dropped) this.fireAll();
      this.dropped = false;
      this.setConnected(true);
    };
    source.onerror = () => {
      this.dropped = true;
      this.setConnected(false);
      // The browser retries a dropped stream itself, not a refused one (a
      // 401, a proxy between restarts): pages read now, and it is tried again.
      if (source.readyState === EventSource.CLOSED && this.source === source) {
        this.fireAll();
        this.win.clearTimeout(this.retry);
        this.retry = this.win.setTimeout(() => {
          if (this.source === source) {
            this.source = null;
            this.open();
          }
        }, RETRY_MS);
      }
    };
    for (const t of topics.split(',') as Topic[]) source.addEventListener(t, () => this.fire(t));
    source.addEventListener('resync', () => this.fireAll());
  }

  private close(): void {
    this.win.clearTimeout(this.retry);
    this.source?.close();
    this.source = null;
    this.opened = '';
    this.setConnected(false);
  }

  private setConnected(on: boolean): void {
    this.connected = on;
    // While the stream is down, a floor: pages read every so often anyway.
    this.win.clearInterval(this.floor);
    if (!on && this.wanted.size > 0) this.floor = this.win.setInterval(() => { if (this.visible) this.fireAll(); }, FLOOR_MS);
  }

  private fire(t: Topic): void {
    for (const fn of this.wanted.get(t) ?? []) this.due.add(fn);
    this.tell();
  }

  private fireAll(): void {
    for (const set of this.wanted.values()) for (const fn of set) this.due.add(fn);
    this.tell();
  }

  /** Tells each page due once, whatever number of its topics changed. */
  private tell(): void {
    if (this.gather !== 0) return;
    this.gather = this.win.setTimeout(() => {
      this.gather = 0;
      const due = [...this.due];
      this.due.clear();
      for (const fn of due) fn();
    }, GATHER_MS);
  }
}
