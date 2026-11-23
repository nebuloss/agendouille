import type { Page } from '@/shared/core/Pages.ts';

/** The code a scanned QR carries in the URL fragment ("#c=<code>"), or "". */
export function codeFromHash(hash: string): string {
  const params = new URLSearchParams(hash.startsWith('#') ? hash.slice(1) : hash);
  return (params.get('c') ?? '').trim();
}

/**
 * "Approve a sign-in" (PLAN.md 8.2). A QR code opens this page with the
 * code in the fragment, which never reaches the server; this hands it to
 * the lookup form, so the person goes straight to picking the number. The
 * fragment is cleared so the code does not stay in the history.
 */
export class ApprovePage implements Page {
  constructor(private readonly doc: Document) {}

  start(): void {
    const code = codeFromHash(this.doc.location.hash);
    if (code === '') return;
    this.doc.defaultView?.history.replaceState(null, '', this.doc.location.pathname);
    const form = this.doc.getElementById('approve-lookup');
    const input = form?.querySelector<HTMLInputElement>('input[name="code"]');
    if (!(form instanceof HTMLFormElement) || input == null) return;
    input.value = code;
    form.submit();
  }

  dispose(): void {}
}
