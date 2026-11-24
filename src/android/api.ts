/**
 * The family API from the app's screens: the page's Api over the bridge
 * (MainActivity "api"), which sends the request with the account's session
 * and answers with the server's status and JSON. The screens are served
 * by the app itself, so the server is never reached from the page.
 */
import { Api, type FetchFn } from '@/shared/core/Api.ts';
import type { Bridge } from '@/shared/core/Bridge.ts';

export function bridgeFetch(bridge: Bridge): FetchFn {
  return async (input, init) => {
    const raw = init.body;
    if (raw !== undefined && raw !== null && typeof raw !== 'string') throw new Error('only JSON is sent from the app');
    const r = await bridge.call<{ status?: unknown; body?: unknown }>('api', {
      method: init.method ?? 'GET', path: input, body: raw ?? null,
    });
    const status = typeof r?.status === 'number' ? r.status : 502;
    const text = typeof r?.body === 'string' ? r.body : '';
    const empty = status === 204 || status === 304 || text === '';
    return new Response(empty ? null : text, { status, headers: { 'Content-Type': 'application/json' } });
  };
}

/** The family API (/api), through the app. */
export function bridgeApi(bridge: Bridge): Api {
  return new Api('/api', bridgeFetch(bridge));
}
