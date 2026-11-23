import { messages } from '@/shared/i18n/i18n.ts';

// A rule in words. {count} is the interval ("every 2 weeks"); the endings
// wrap the rule ({rule}), so each language puts them where it wants.
export const t = messages({
  fr: {
    never: 'Ne se répète pas',
    advanced: 'Se répète (règle avancée)',
    daily: { one: 'Chaque jour', other: 'Tous les {count} jours' },
    weekly: { one: 'Chaque semaine', other: 'Toutes les {count} semaines' },
    weeklyOnDay: { one: 'Chaque semaine le {day}', other: 'Toutes les {count} semaines le {day}' },
    weeklyOnDays: { one: 'Chaque semaine les {days}', other: 'Toutes les {count} semaines les {days}' },
    monthly: { one: 'Chaque mois', other: 'Tous les {count} mois' },
    yearly: { one: 'Chaque année', other: 'Tous les {count} ans' },
    until: '{rule}, jusqu’au {date}',
    times: { one: '{rule}, une fois', other: '{rule}, {count} fois' },
  },
  en: {
    never: 'Does not repeat',
    advanced: 'Repeats (advanced rule)',
    daily: { one: 'Every day', other: 'Every {count} days' },
    weekly: { one: 'Every week', other: 'Every {count} weeks' },
    weeklyOnDay: { one: 'Every week on {day}', other: 'Every {count} weeks on {day}' },
    weeklyOnDays: { one: 'Every week on {days}', other: 'Every {count} weeks on {days}' },
    monthly: { one: 'Every month', other: 'Every {count} months' },
    yearly: { one: 'Every year', other: 'Every {count} years' },
    until: '{rule}, until {date}',
    times: { one: '{rule}, once', other: '{rule}, {count} times' },
  },
});
