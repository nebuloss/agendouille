/**
 * Field errors in the page's design and language, in place of the
 * browser's bubbles ("Veuillez renseigner ce champ."): the browser still
 * checks (required, type, pattern, lengths), the page says why, under the
 * field, and the field is marked. One listener for the whole page: every
 * form, a dialog's or a server-rendered one.
 */
import { h } from '@/shared/core/Dom.ts';
import { t } from '@/shared/components/Validation.msg.ts';

type Field = HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;

/** The sentence for a field's first problem. */
export function reason(f: Field): string {
  const v = f.validity;
  if (v.valueMissing) return f instanceof HTMLInputElement && (f.type === 'checkbox' || f.type === 'radio') ? t('check') : t('required');
  if (v.typeMismatch) return f instanceof HTMLInputElement && f.type === 'email' ? t('email') : f instanceof HTMLInputElement && f.type === 'url' ? t('url') : t('format');
  if (v.patternMismatch) return f.title !== '' ? f.title : t('format');
  if (v.tooShort) return t('tooShort', { n: (f as HTMLInputElement).minLength });
  if (v.tooLong) return t('tooLong', { n: (f as HTMLInputElement).maxLength });
  if (v.rangeUnderflow) return t('min', { n: (f as HTMLInputElement).min });
  if (v.rangeOverflow) return t('max', { n: (f as HTMLInputElement).max });
  if (v.badInput || v.stepMismatch) return t('format');
  return f.validationMessage;
}

function messageOf(f: Field): HTMLElement {
  const id = f.getAttribute('aria-describedby');
  const existing = id !== null ? f.ownerDocument.getElementById(id) : null;
  if (existing !== null && existing.classList.contains('field-error')) return existing;
  const m = h('p', { class: 'field-error m-0 text-xs font-medium text-error', id: `err-${Math.random().toString(36).slice(2)}`, role: 'alert' });
  f.setAttribute('aria-describedby', m.id);
  f.insertAdjacentElement('afterend', m);
  return m;
}

function clear(f: Field): void {
  f.removeAttribute('aria-invalid');
  const id = f.getAttribute('aria-describedby');
  const m = id !== null ? f.ownerDocument.getElementById(id) : null;
  if (m !== null && m.classList.contains('field-error')) m.remove();
}

/** Installs the page's validation messages; call once per page. */
export function ownValidation(doc: Document = document): void {
  let first = true;
  doc.addEventListener('invalid', (e) => {
    const f = e.target as Field;
    e.preventDefault(); // no browser bubble
    f.setAttribute('aria-invalid', 'true');
    messageOf(f).textContent = reason(f);
    if (first) {
      f.focus();
      first = false;
      queueMicrotask(() => { first = true; });
    }
  }, true);
  const recheck = (e: Event): void => {
    const f = e.target as Field;
    if (f.getAttribute?.('aria-invalid') === 'true' && f.checkValidity?.()) clear(f);
  };
  doc.addEventListener('input', recheck, true);
  doc.addEventListener('change', recheck, true);
}
