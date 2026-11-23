/**
 * A QR code scanner in a dialog (PLAN.md 7.4: the scanner is in the page,
 * the app only grants the camera). The browser's BarcodeDetector when it
 * has one (Chromium with Google Play services); jsQR otherwise, loaded only
 * then, so a phone without Google's services scans too.
 */
import { h } from '@/shared/core/Dom.ts';
import { ui } from '@/shared/components/recipes.ts';
import { t } from '@/app/components/Scanner.msg.ts';

interface Detector {
  detect(source: CanvasImageSource): Promise<{ rawValue: string }[]>;
}
type DetectorCtor = new (o: { formats: string[] }) => Detector;

/** The approval code a sign-in QR carries: https://<our host>/approve#c=<code>. */
export function approvalTarget(text: string, origin: string): string | null {
  try {
    const u = new URL(text.trim());
    if (u.origin !== origin || u.pathname !== '/approve') return null;
    const code = new URLSearchParams(u.hash.slice(1)).get('c');
    return code !== null && /^[A-Za-z0-9_-]{4,64}$/.test(code) ? `/approve#c=${encodeURIComponent(code)}` : null;
  } catch {
    return null;
  }
}

/** Opens the camera and resolves with the first QR code read, or null when closed. */
export function scanQR(doc: Document): Promise<string | null> {
  const el = doc.createElement('dialog');
  el.className = ui.dialog;
  const video = h('video', { class: 'aspect-square w-full rounded-md3-l bg-black object-cover', playsinline: true, muted: true, autoplay: true });
  const status = h('p', { class: 'text-sm text-on-surface-variant', role: 'status' }, t('starting'));
  const close = h('button', { type: 'button', class: ui.btn('text') }, t('cancel'));
  el.append(h('div', { class: 'flex max-h-[inherit] flex-col' },
    h('div', { class: ui.dialogHead }, h('h2', {}, t('title'))),
    h('div', { class: ui.dialogBody }, video, status),
    h('div', { class: ui.dialogFoot }, close)));
  doc.body.append(el);

  let stream: MediaStream | null = null;
  let stopped = false;
  let result: string | null = null;
  const stop = (): void => {
    stopped = true;
    stream?.getTracks().forEach((tr) => tr.stop());
  };

  return new Promise((resolve) => {
    el.addEventListener('close', () => {
      stop();
      el.remove();
      resolve(result);
    }, { once: true });
    close.addEventListener('click', () => el.close());
    if (typeof el.showModal === 'function') el.showModal();
    else el.setAttribute('open', '');

    const start = async (): Promise<void> => {
      const media = doc.defaultView?.navigator.mediaDevices;
      if (media === undefined) throw new Error(t('noCamera'));
      stream = await media.getUserMedia({ video: { facingMode: 'environment' }, audio: false });
      video.srcObject = stream;
      await video.play();
      status.textContent = t('aim');
      const Native = (doc.defaultView as unknown as { BarcodeDetector?: DetectorCtor }).BarcodeDetector;
      let native: Detector | null = null;
      if (Native !== undefined) {
        try {
          native = new Native({ formats: ['qr_code'] });
        } catch {
          native = null;
        }
      }
      const jsQR = native === null ? (await import('jsqr')).default : null;
      const canvas = doc.createElement('canvas');
      const ctx = canvas.getContext('2d', { willReadFrequently: true });
      const frame = async (): Promise<void> => {
        if (stopped) return;
        let text: string | null = null;
        if (video.videoWidth > 0) {
          if (native !== null) {
            const found = await native.detect(video).catch(() => []);
            text = found[0]?.rawValue ?? null;
          } else if (jsQR !== null && ctx !== null) {
            // A smaller copy of the frame: faster, and enough for a QR code.
            const scale = Math.min(1, 640 / Math.max(video.videoWidth, video.videoHeight));
            canvas.width = Math.round(video.videoWidth * scale);
            canvas.height = Math.round(video.videoHeight * scale);
            ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
            const img = ctx.getImageData(0, 0, canvas.width, canvas.height);
            text = jsQR(img.data, img.width, img.height, { inversionAttempts: 'dontInvert' })?.data ?? null;
          }
        }
        if (text !== null && text !== '') {
          result = text;
          el.close();
          return;
        }
        doc.defaultView?.setTimeout(() => void frame(), 120);
      };
      void frame();
    };
    start().catch((e: unknown) => {
      status.textContent = e instanceof Error && e.name === 'NotAllowedError' ? t('denied') : e instanceof Error ? e.message : String(e);
      status.className = 'text-sm font-semibold text-error';
    });
  });
}
