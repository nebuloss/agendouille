import { messages } from '@/shared/i18n/i18n.ts';

export const t = messages({
  fr: {
    created: 'Créés',
    changed: 'Modifiés',
    deleted: 'Supprimés',
    chartLabel: {
      one: 'Modifications par jour sur {count} jour : {created} créés, {changed} modifiés, {deleted} supprimés',
      other: 'Modifications par jour sur {count} jours : {created} créés, {changed} modifiés, {deleted} supprimés',
    },
    seriesValue: '{label} : {value}',
    showFigures: 'Afficher les chiffres',
    noChanges: 'Aucune modification sur cette période.',
    day: 'Jour',
  },
  en: {
    created: 'Created',
    changed: 'Changed',
    deleted: 'Deleted',
    chartLabel: {
      one: 'Changes per day over {count} day: {created} created, {changed} changed, {deleted} deleted',
      other: 'Changes per day over {count} days: {created} created, {changed} changed, {deleted} deleted',
    },
    seriesValue: '{label}: {value}',
    showFigures: 'Show the figures',
    noChanges: 'No changes in this period.',
    day: 'Day',
  },
});
