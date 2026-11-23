import { Api } from '@/shared/core/Api.ts';

/** The family web app's API (CONTRACTS.md 3.2 to 3.4). */
export function appApi(fetchFn?: (input: string, init: RequestInit) => Promise<Response>): Api {
  return new Api('/api', fetchFn);
}

export interface Share {
  target: string; // u:lea, g:famille
  access: 'read' | 'readwrite';
}

export interface Calendar {
  ref: string;
  owner: string;
  name: string;
  color: string;
  timezone: string;
  description: string;
  access: 'read' | 'readwrite' | 'owner';
  shown: boolean;
  hidden: boolean;
  seq: number;
  shares: Share[] | null;
}

export interface Calendars {
  family_timezone: string;
  calendars: Calendar[];
}

export interface Occurrence {
  calendar: string;
  name: string;
  etag: string;
  uid: string;
  recurrence_id: string | null;
  summary: string;
  location: string;
  all_day: boolean;
  start: string;
  end: string;
  recurring: boolean;
  overridden: boolean;
  created_by: string;
  modified_by: string;
}

export interface EventView {
  calendar: string;
  name: string;
  etag: string;
  uid: string;
  summary: string;
  location: string;
  description: string;
  all_day: boolean;
  start: string;
  end: string;
  rrule: string;
  exdates: string[];
  has_alarms: boolean;
  has_attendees: boolean;
  editable_rrule: boolean;
  created_by: string;
  created_at: string;
  modified_by: string;
  modified_at: string;
  writable: boolean;
}

export interface EventInput {
  summary: string;
  location: string;
  description: string;
  all_day: boolean;
  start: string;
  end: string;
  rrule?: string | null;
}

export interface Person {
  name: string;
  display_name: string;
  color: string;
}

export interface Me extends Person {
  groups: { name: string; display_name: string }[];
}

export type Scope = 'this' | 'following' | 'all';

export const canWrite = (c: Calendar): boolean => c.access === 'owner' || c.access === 'readwrite';
