/**
 * The admin's one stream of notices (GET /api/admin/events, internal/live),
 * and what each page is computed from: the topics it reads again on
 * (claudication's READS). A page that names fewer goes stale on the one left
 * out.
 */
import { LiveStream, type Topic } from '@/shared/core/Live.ts';

export const liveStream = new LiveStream(window, '/api/admin/events');

export const READS = {
  overview: ['people', 'devices', 'calendars', 'settings', 'backups'],
  users: ['people', 'devices', 'calendars'],
  user: ['people', 'devices', 'calendars', 'activity', 'settings'],
  groups: ['people', 'calendars'],
  group: ['people', 'calendars', 'activity'],
  calendars: ['calendars', 'people'],
  calendar: ['calendars', 'people', 'activity'],
  activity: ['activity'],
  settings: ['settings', 'devices'],
  backups: ['backups'],
  notices: ['people', 'devices', 'settings', 'backups', 'notices'],
} as const satisfies Record<string, readonly Topic[]>;
