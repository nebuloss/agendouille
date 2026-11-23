/**
 * The bridge to the Android app (CONTRACTS.md 6), version 2. The app posts
 * a MessagePort with the message "agendouille-bridge" after each page load;
 * with none within a moment after load, the page is in a plain browser and
 * connect() resolves null. Calls are JSON, answered by id; the app may also
 * send events ({"event", "data"}).
 */

export const PAGE_BRIDGE = 2;
const WAIT_AFTER_LOAD_MS = 1500;

export interface Hello {
  app_bridge: number;
  app_version: string;
  android: number;
}

export class BridgeError extends Error {
  /** code is for the page to test; detail, when there is one, is the app's
   * sentence for a person, already in the phone's language. */
  constructor(readonly code: string, readonly detail?: string) {
    super(detail ?? code);
    this.name = 'BridgeError';
  }
}

type Waiter = { resolve: (v: unknown) => void; reject: (e: Error) => void };

export class Bridge {
  private next = 1;
  private readonly waiting = new Map<number, Waiter>();
  private readonly listeners = new Map<string, ((data: unknown) => void)[]>();
  hello: Hello | null = null;

  constructor(private readonly port: MessagePort) {
    port.onmessage = (e: MessageEvent) => this.receive(e.data);
  }

  /** Calls the app; resolves with its result, rejects with a BridgeError. */
  call<T = unknown>(name: string, args?: Record<string, unknown>): Promise<T> {
    const id = this.next++;
    return new Promise<T>((resolve, reject) => {
      this.waiting.set(id, { resolve: resolve as (v: unknown) => void, reject });
      this.port.postMessage(JSON.stringify(args === undefined ? { id, call: name } : { id, call: name, args }));
    });
  }

  /** Listens to an event the app sends on its own ("sync.changed"). */
  on(event: string, fn: (data: unknown) => void): void {
    const list = this.listeners.get(event) ?? [];
    list.push(fn);
    this.listeners.set(event, list);
  }

  private receive(raw: unknown): void {
    if (typeof raw !== 'string') return;
    let m: Record<string, unknown>;
    try {
      const v: unknown = JSON.parse(raw);
      if (v === null || typeof v !== 'object') return;
      m = v as Record<string, unknown>;
    } catch {
      return;
    }
    if (typeof m['event'] === 'string') {
      for (const fn of this.listeners.get(m['event']) ?? []) fn(m['data']);
      return;
    }
    const id = typeof m['id'] === 'number' ? m['id'] : -1;
    const w = this.waiting.get(id);
    if (w === undefined) return;
    this.waiting.delete(id);
    if (m['ok'] === true) w.resolve(m['result']);
    else w.reject(new BridgeError(typeof m['error'] === 'string' ? m['error'] : 'error', typeof m['message'] === 'string' ? m['message'] : undefined));
  }
}

let connecting: Promise<Bridge | null> | null = null;

/**
 * The app's bridge, or null in a plain browser. The listener is set when
 * this module loads, before the app posts its port.
 */
export function bridge(win: Window = window): Promise<Bridge | null> {
  connecting ??= new Promise<Bridge | null>((resolve) => {
    let done = false;
    const finish = (b: Bridge | null): void => {
      if (done) return;
      done = true;
      resolve(b);
    };
    win.addEventListener('message', (e: MessageEvent) => {
      if (e.data !== 'agendouille-bridge' || e.ports.length === 0) return;
      const b = new Bridge(e.ports[0]!);
      b.call<Hello>('hello', { page_bridge: PAGE_BRIDGE }).then((h) => {
        b.hello = h;
        finish(b);
      }, () => finish(null));
    });
    const wait = (): void => {
      win.setTimeout(() => finish(null), WAIT_AFTER_LOAD_MS);
    };
    if (win.document.readyState === 'complete') wait();
    else win.addEventListener('load', wait, { once: true });
  });
  return connecting;
}

/** For the tests: forget the connection. */
export function resetBridge(): void {
  connecting = null;
}
