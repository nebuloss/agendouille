import { afterEach, describe, expect, it } from 'vitest';
import { Bridge, BridgeError, bridge, resetBridge } from '@/shared/core/Bridge.ts';

/** A fake app at the other end of a MessageChannel. */
function app(answer: (req: { id: number; call: string; args?: Record<string, unknown> }) => unknown): { port: MessagePort; sent: unknown[] } {
  const ch = new MessageChannel();
  const sent: unknown[] = [];
  ch.port2.onmessage = (e: MessageEvent) => {
    const req = JSON.parse(e.data as string) as { id: number; call: string; args?: Record<string, unknown> };
    sent.push(req);
    const out = answer(req);
    if (out !== undefined) ch.port2.postMessage(JSON.stringify(out));
  };
  return { port: ch.port1, sent };
}

const tick = (ms = 20): Promise<void> => new Promise((r) => setTimeout(r, ms));

describe('Bridge', () => {
  afterEach(() => resetBridge());

  it('answers calls by id, and rejects with the app’s error', async () => {
    const a = app((r) => (r.call === 'scan' ? { id: r.id, ok: false, error: 'camera_denied' } : { id: r.id, ok: true, result: { n: r.id } }));
    const b = new Bridge(a.port);
    expect(await b.call('sync.status')).toEqual({ n: 1 });
    await expect(b.call('scan')).rejects.toEqual(new BridgeError('camera_denied'));
    expect(a.sent).toEqual([{ id: 1, call: 'sync.status' }, { id: 2, call: 'scan' }]);
  });

  it('passes events to their listeners and ignores junk', async () => {
    const ch = new MessageChannel();
    const b = new Bridge(ch.port1);
    const seen: unknown[] = [];
    b.on('sync.changed', (d) => seen.push(d));
    ch.port2.postMessage('not json');
    ch.port2.postMessage(JSON.stringify({ id: 99, ok: true }));
    ch.port2.postMessage(JSON.stringify({ event: 'sync.changed', data: { state: 'syncing' } }));
    await tick();
    expect(seen).toEqual([{ state: 'syncing' }]);
  });

  it('connects when the app posts its port, says hello, and is null in a browser', async () => {
    const a = app((r) => ({ id: r.id, ok: true, result: { app_bridge: 1, app_version: '0.6.0', android: 34 } }));
    const p = bridge(window);
    window.dispatchEvent(new MessageEvent('message', { data: 'agendouille-bridge', ports: [a.port] }));
    const b = await p;
    expect(b?.hello?.app_version).toBe('0.6.0');
    expect(a.sent[0]).toEqual({ id: 1, call: 'hello', args: { page_bridge: 2 } });
  });
});
