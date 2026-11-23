import { messages } from '@/shared/i18n/i18n.ts';

export const t = messages({
  fr: {
    addedBy: 'Ajouté par {name}',
    nothingToday: 'Rien de prévu aujourd’hui.',
    newEvent: 'Nouvel événement',
    newEventOn: 'Nouvel événement le {day}',
    nothingInRange: 'Aucun événement sur cette période.',
    allDay: 'jour',
    // When an occurrence is, as the sheet says it.
    dayRange: 'du {from} au {to}',
    timeRange: '{day}, {start} – {end}',
    acrossDays: '{startDay} {start} – {endDay} {end}',
  },
  en: {
    addedBy: 'Added by {name}',
    nothingToday: 'Nothing planned today.',
    newEvent: 'New event',
    newEventOn: 'New event on {day}',
    nothingInRange: 'No events in this period.',
    allDay: 'all day',
    dayRange: '{from} to {to}',
    timeRange: '{day}, {start} – {end}',
    acrossDays: '{startDay} {start} – {endDay} {end}',
  },
});
