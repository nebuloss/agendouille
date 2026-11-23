/**
 * Line icons, drawn in the current colour (cafard's set, plus a calendar's).
 * Built as SVG elements, never as markup: no HTML sink.
 */
const PATHS = {
  calendar: 'M4 7a2 2 0 012-2h12a2 2 0 012 2v12a2 2 0 01-2 2H6a2 2 0 01-2-2z M4 10h16 M8 3v4 M16 3v4',
  calendars: 'M3 8a2 2 0 012-2h10a2 2 0 012 2v10a2 2 0 01-2 2H5a2 2 0 01-2-2z M3 11h14 M7 4v4 M13 4v4 M20 9v9a3 3 0 01-3 3H8',
  devices: 'M4 6a2 2 0 012-2h10a2 2 0 012 2v2 M2 18h11 M4 6v9 M15 10a1 1 0 011-1h5a1 1 0 011 1v10a1 1 0 01-1 1h-5a1 1 0 01-1-1z M18 18h1',
  person: 'M12 11.5a3.5 3.5 0 100-7 3.5 3.5 0 000 7z M5 20c.8-3.8 3.6-6 7-6s6.2 2.2 7 6',
  people: 'M9 11.2a3.2 3.2 0 100-6.4 3.2 3.2 0 000 6.4z M3 19c.6-3.3 3-5 6-5s5.4 1.7 6 5 M16 5.5a3 3 0 010 5.8 M18 14c1.7.6 2.7 2.4 3 5',
  group: 'M12 10a3 3 0 100-6 3 3 0 000 6z M6 20c.5-3 2.8-5 6-5s5.5 2 6 5 M5 9.5a2.2 2.2 0 100-4.4 M19 9.5a2.2 2.2 0 110-4.4',
  home: 'M4 11l8-7 8 7 M6 9.5V20h4.5v-5h3v5H18V9.5',
  list: 'M8 6h12 M8 12h12 M8 18h12 M4 6h.01 M4 12h.01 M4 18h.01',
  gear: 'M12 15a3 3 0 100-6 3 3 0 000 6z M12 2v3 M12 19v3 M4.2 4.2l2.1 2.1 M17.7 17.7l2.1 2.1 M2 12h3 M19 12h3 M4.2 19.8l2.1-2.1 M17.7 6.3l2.1-2.1',
  disk: 'M4 6c0-1.7 3.6-3 8-3s8 1.3 8 3-3.6 3-8 3-8-1.3-8-3z M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6 M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3',
  plus: 'M12 5v14 M5 12h14',
  close: 'M6 6l12 12 M18 6L6 18',
  left: 'M15 5l-7 7 7 7',
  right: 'M9 5l7 7-7 7',
  today: 'M4 7a2 2 0 012-2h12a2 2 0 012 2v12a2 2 0 01-2 2H6a2 2 0 01-2-2z M4 10h16 M8 3v4 M16 3v4 M12 16.5h.01',
  edit: 'M4 20h4L19 9l-4-4L4 16z M13.5 6.5l4 4',
  trash: 'M4 7h16 M9 7V4h6v3 M6 7l1 13h10l1-13 M10 11v6 M14 11v6',
  share: 'M18 8a3 3 0 100-6 3 3 0 000 6z M6 15a3 3 0 100-6 3 3 0 000 6z M18 22a3 3 0 100-6 3 3 0 000 6z M8.6 13.5l6.8 4 M15.4 6.5l-6.8 4',
  repeat: 'M17 2l4 4-4 4 M3 11V9a3 3 0 013-3h15 M7 22l-4-4 4-4 M21 13v2a3 3 0 01-3 3H3',
  pin: 'M12 21s-7-6.2-7-11.5A7 7 0 0112 2.5a7 7 0 017 7C19 14.8 12 21 12 21z M12 12a2.5 2.5 0 100-5 2.5 2.5 0 000 5z',
  clock: 'M12 21a9 9 0 100-18 9 9 0 000 18z M12 7v5l3 2',
  bell: 'M6 16v-5a6 6 0 0112 0v5l1.5 2h-15z M10 20.5a2 2 0 004 0',
  notes: 'M5 4h14v16H5z M8 8h8 M8 12h8 M8 16h5',
  download: 'M12 4v11 M7 10l5 5 5-5 M4 20h16',
  upload: 'M12 20V9 M7 14l5-5 5 5 M4 4h16',
  hide: 'M3 3l18 18 M10.6 6.1A9.8 9.8 0 0112 6c5 0 9 6 9 6a16 16 0 01-2.6 3.2 M6.6 6.6C4.2 8.2 3 12 3 12s4 6 9 6c1.6 0 3-.4 4.3-1',
  show: 'M3 12s4-6 9-6 9 6 9 6-4 6-9 6-9-6-9-6z M12 14.5a2.5 2.5 0 100-5 2.5 2.5 0 000 5z',
  give: 'M4 12h12 M12 8l4 4-4 4 M20 4v16',
  link: 'M10 14a4 4 0 005.7 0l3-3a4 4 0 00-5.7-5.7l-1 1 M14 10a4 4 0 00-5.7 0l-3 3a4 4 0 005.7 5.7l1-1',
  phone: 'M7 3h10a1 1 0 011 1v16a1 1 0 01-1 1H7a1 1 0 01-1-1V4a1 1 0 011-1z M11 18h2',
  laptop: 'M5 5h14v10H5z M2 19h20',
  qr: 'M4 4h6v6H4z M14 4h6v6h-6z M4 14h6v6H4z M14 14h2v2h-2z M18 18h2v2h-2z M18 14h2 M14 18v2',
  logout: 'M15 4h4v16h-4 M10 8l-4 4 4 4 M6 12h11',
  warn: 'M12 4l9 16H3z M12 10v4 M12 17v.5',
  info: 'M12 21a9 9 0 100-18 9 9 0 000 18z M12 11v5 M12 8v.5',
  check: 'M5 12.5l4.5 4.5L19 7.5',
  lock: 'M5 11h14v9H5z M8 11V8a4 4 0 018 0v3',
  search: 'M11 17a6 6 0 100-12 6 6 0 000 12z M20 20l-4.5-4.5',
  palette: 'M12 21a9 9 0 110-18c5 0 9 3.6 9 8 0 2.8-2.2 4-4 4h-2a2 2 0 00-1.5 3.3A2 2 0 0112 21z M7.5 11h.01 M10 7h.01 M14.5 7h.01',
} as const;

export type IconName = keyof typeof PATHS;

const NS = 'http://www.w3.org/2000/svg';

/** An icon as an SVG element, aria-hidden: its button or link has the label. */
export function icon(name: IconName, doc: Document = document): SVGSVGElement {
  const svg = doc.createElementNS(NS, 'svg');
  svg.setAttribute('class', 'ic');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('aria-hidden', 'true');
  const path = doc.createElementNS(NS, 'path');
  path.setAttribute('d', PATHS[name]);
  svg.append(path);
  return svg;
}
