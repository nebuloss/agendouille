/**
 * The flags of the languages offered, as small SVG elements (no markup:
 * no HTML sink). A flag stands for a language here, beside its name, never
 * alone: French is not only France's, but the flag is what people scan for.
 */
import type { Lang } from '@/shared/i18n/i18n.ts';

const NS = 'http://www.w3.org/2000/svg';

type Shape = readonly [tag: 'rect' | 'path', attrs: Readonly<Record<string, string>>];

// viewBox 0 0 30 20 (3:2), drawn in a rounded clip by flag().
const SHAPES: Readonly<Record<Lang, readonly Shape[]>> = {
  fr: [
    ['rect', { width: '10', height: '20', fill: '#002654' }],
    ['rect', { x: '10', width: '10', height: '20', fill: '#ffffff' }],
    ['rect', { x: '20', width: '10', height: '20', fill: '#ce1126' }],
  ],
  // The Union flag, simplified to read at 20 px: the saltires, then the cross.
  en: [
    ['rect', { width: '30', height: '20', fill: '#012169' }],
    ['path', { d: 'M0 0L30 20M30 0L0 20', stroke: '#ffffff', 'stroke-width': '4' }],
    ['path', { d: 'M0 0L30 20M30 0L0 20', stroke: '#c8102e', 'stroke-width': '1.6' }],
    ['path', { d: 'M15 0v20M0 10h30', stroke: '#ffffff', 'stroke-width': '6' }],
    ['path', { d: 'M15 0v20M0 10h30', stroke: '#c8102e', 'stroke-width': '3.4' }],
  ],
};

let clipIds = 0;

/** A language's flag, 3:2, with rounded corners and a hairline edge. */
export function flag(lang: Lang, height = 14, doc: Document = document): SVGSVGElement {
  const svg = doc.createElementNS(NS, 'svg');
  svg.setAttribute('viewBox', '0 0 30 20');
  svg.setAttribute('width', String(Math.round(height * 1.5)));
  svg.setAttribute('height', String(height));
  svg.setAttribute('aria-hidden', 'true');
  svg.setAttribute('class', 'flag shrink-0');
  const id = `flag-clip-${++clipIds}`;
  const defs = doc.createElementNS(NS, 'defs');
  const clip = doc.createElementNS(NS, 'clipPath');
  clip.setAttribute('id', id);
  const r = doc.createElementNS(NS, 'rect');
  for (const [k, v] of Object.entries({ width: '30', height: '20', rx: '3' })) r.setAttribute(k, v);
  clip.append(r);
  defs.append(clip);
  const g = doc.createElementNS(NS, 'g');
  g.setAttribute('clip-path', `url(#${id})`);
  for (const [tag, attrs] of SHAPES[lang]) {
    const el = doc.createElementNS(NS, tag);
    for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
    g.append(el);
  }
  const edge = doc.createElementNS(NS, 'rect');
  for (const [k, v] of Object.entries({ x: '0.5', y: '0.5', width: '29', height: '19', rx: '2.5', fill: 'none', stroke: 'currentColor', 'stroke-opacity': '0.25' })) edge.setAttribute(k, v);
  svg.append(defs, g, edge);
  return svg;
}
