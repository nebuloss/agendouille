/**
 * An error the server described (CONTRACTS.md 3.1), or a failure to reach
 * it. message is the server's own sentence and is shown as it is.
 */
import { t } from '@/shared/core/Api.msg.ts';

export class ApiError extends Error {
  constructor(
    readonly code: string,
    message: string,
    readonly status: number,
    readonly body: Record<string, unknown> = {},
  ) {
    super(message);
    this.name = 'ApiError';
  }

  static offline(): ApiError {
    return new ApiError('offline', t('offline'), 0);
  }

  static from(raw: unknown, status: number): ApiError {
    if (raw !== null && typeof raw === 'object') {
      const o = raw as Record<string, unknown>;
      const code = typeof o['error'] === 'string' ? o['error'] : 'http_error';
      const message = typeof o['message'] === 'string' && o['message'] !== '' ? o['message'] : t('httpError', { status });
      return new ApiError(code, message, status, o);
    }
    return new ApiError('http_error', t('httpError', { status }), status);
  }
}

export type FetchFn = (input: string, init: RequestInit) => Promise<Response>;

/**
 * JSON over fetch, same-origin only. Pages never call fetch themselves.
 * CSRF is refused by the server on Sec-Fetch-Site, which the browser sets.
 */
export class Api {
  constructor(
    private readonly base: string,
    private readonly fetchFn: FetchFn = (input, init) => globalThis.fetch(input, init),
  ) {
    if (!base.startsWith('/') || base.startsWith('//')) throw new Error(`not a same-origin path: ${base}`);
  }

  get<T>(path: string, query?: Record<string, string | undefined>): Promise<T> {
    return this.request<T>('GET', path + queryString(query));
  }

  post<T>(path: string, body: unknown = {}, ifMatch?: string): Promise<T> {
    return this.request<T>('POST', path, body, ifMatch);
  }

  /** PUT, with If-Match when given: a 412 means someone changed it meanwhile. */
  put<T>(path: string, body: unknown, ifMatch?: string, query?: Record<string, string | undefined>): Promise<T> {
    return this.request<T>('PUT', path + queryString(query), body, ifMatch);
  }

  patch<T>(path: string, body: unknown): Promise<T> {
    return this.request<T>('PATCH', path, body);
  }

  delete(path: string, query?: Record<string, string | undefined>): Promise<void> {
    return this.request<void>('DELETE', path + queryString(query));
  }

  /** DELETE with If-Match. */
  deleteIf(path: string, ifMatch: string, query?: Record<string, string | undefined>): Promise<void> {
    return this.request<void>('DELETE', path + queryString(query), undefined, ifMatch);
  }

  /** Sends a file's bytes as they are (an .ics import). */
  upload<T>(path: string, body: Blob, query?: Record<string, string | undefined>): Promise<T> {
    return this.request<T>('POST', path + queryString(query), body);
  }

  /** The URL of path, for links (downloads). */
  url(path: string): string {
    return this.base + path;
  }

  private async request<T>(method: string, path: string, body?: unknown, ifMatch?: string): Promise<T> {
    const headers: Record<string, string> = { Accept: 'application/json' };
    const init: RequestInit = { method, headers, credentials: 'same-origin', redirect: 'error' };
    if (ifMatch !== undefined) headers['If-Match'] = ifMatch;
    if (body instanceof Blob) {
      headers['Content-Type'] = body.type || 'text/calendar';
      init.body = body;
    } else if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }
    let resp: Response;
    try {
      resp = await this.fetchFn(this.base + path, init);
    } catch {
      throw ApiError.offline();
    }
    if (resp.status === 204) return undefined as T;
    let raw: unknown = null;
    try {
      raw = await resp.json();
    } catch {
      raw = null;
    }
    if (!resp.ok) throw ApiError.from(raw, resp.status);
    return raw as T;
  }
}

function queryString(query?: Record<string, string | undefined>): string {
  if (query === undefined) return '';
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v !== undefined && v !== '') params.set(k, v);
  }
  const s = params.toString();
  return s === '' ? '' : `?${s}`;
}

/** Encodes one path segment (a user name, a calendar ref, a file). */
export function seg(s: string): string {
  return encodeURIComponent(s);
}
