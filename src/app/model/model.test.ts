import { describe, expect, it } from 'vitest';
import {
  addDays, addMonths, daysBetween, formatLocal, nowIn, parseLocal, startOfWeek, weekday, labelDay, weekdayNames,
} from '@/shared/core/Dates.ts';
import { describe as words, fromRule, toRule, none } from '@/app/model/Recurrence.ts';

describe('Dates', () => {
  it('parses and formats wall times', () => {
    expect(formatLocal(parseLocal('2026-11-25T14:00'))).toBe('2026-11-25T14:00:00');
    expect(formatLocal(parseLocal('2026-11-25'))).toBe('2026-11-25T00:00:00');
    expect(() => parseLocal('25/11/2026')).toThrow();
  });

  it('adds days across the end of summer time without moving the hour', () => {
    // 25 October 2026: Paris goes from +02:00 to +01:00.
    const before = parseLocal('2026-10-24T09:00:00');
    expect(formatLocal(addDays(before, 1))).toBe('2026-10-25T09:00:00');
    expect(formatLocal(addDays(before, 2))).toBe('2026-10-26T09:00:00');
    expect(daysBetween(before, parseLocal('2026-10-26T08:00:00'))).toBe(2);
  });

  it('reads now in the family zone, across the change', () => {
    // 00:30 UTC is 02:30 CEST; an hour later the clock went back: 02:30 CET.
    expect(formatLocal(nowIn('Europe/Paris', Date.UTC(2026, 9, 25, 0, 30)))).toBe('2026-10-25T02:30:00');
    expect(formatLocal(nowIn('Europe/Paris', Date.UTC(2026, 9, 25, 1, 30)))).toBe('2026-10-25T02:30:00');
    expect(formatLocal(nowIn('America/Montreal', Date.UTC(2026, 10, 23, 3, 0)))).toBe('2026-11-22T22:00:00');
  });

  it('weeks start on Monday', () => {
    const wed = parseLocal('2026-11-25');
    expect(weekday(wed)).toBe(2);
    expect(formatLocal(startOfWeek(wed))).toBe('2026-11-23T00:00:00');
    expect(formatLocal(startOfWeek(parseLocal('2026-11-29')))).toBe('2026-11-23T00:00:00'); // Sunday
    expect(formatLocal(addMonths(parseLocal('2026-01-31'), 1))).toBe('2026-02-01T00:00:00');
    expect(labelDay(wed)).toBe('mercredi 25 novembre');
    expect(weekdayNames()[0]).toBe('lun.');
  });
});

describe('Recurrence', () => {
  it('reads the rules the editor can show, and refuses the others', () => {
    expect(fromRule('FREQ=WEEKLY;BYDAY=WE;UNTIL=20270630T215959Z')).toMatchObject({ freq: 'weekly', days: ['WE'], end: 'until', until: '2027-06-30' });
    expect(fromRule('FREQ=MONTHLY;COUNT=6')).toMatchObject({ freq: 'monthly', end: 'count', count: 6 });
    expect(fromRule('FREQ=DAILY;INTERVAL=2')).toMatchObject({ freq: 'daily', interval: 2, end: 'never' });
    expect(fromRule('')).toEqual(none());
    expect(fromRule('FREQ=MONTHLY;BYDAY=1FR')).toBeNull();
    expect(fromRule('FREQ=YEARLY;BYWEEKNO=20')).toBeNull();
    expect(fromRule('FREQ=FORTNIGHTLY')).toBeNull();
  });

  it('writes them back', () => {
    expect(toRule(none())).toBeNull();
    expect(toRule({ ...none(), freq: 'weekly', days: ['FR', 'MO'], end: 'until', until: '2027-06-30' })).toBe('FREQ=WEEKLY;BYDAY=MO,FR;UNTIL=20270630');
    expect(toRule({ ...none(), freq: 'daily', interval: 3, end: 'count', count: 5 })).toBe('FREQ=DAILY;INTERVAL=3;COUNT=5');
    const r = 'FREQ=WEEKLY;INTERVAL=2;BYDAY=TU,TH;COUNT=8';
    expect(toRule(fromRule(r)!)).toBe(r);
  });

  it('says them in words', () => {
    expect(words('FREQ=WEEKLY;BYDAY=WE;UNTIL=20270630T215959Z')).toBe("Chaque semaine le mercredi, jusqu'au 30 juin 2027");
    expect(words('FREQ=WEEKLY;BYDAY=TU,TH')).toBe('Chaque semaine les mardi et jeudi');
    expect(words('FREQ=DAILY;INTERVAL=2;COUNT=5')).toBe('Tous les 2 jours, 5 fois');
    expect(words('FREQ=YEARLY')).toBe('Chaque année');
    expect(words(null)).toBe('Ne se répète pas');
    expect(words('FREQ=MONTHLY;BYDAY=-1FR')).toBe('Se répète (règle avancée)');
  });
});
