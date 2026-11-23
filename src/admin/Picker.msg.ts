import { messages } from '@/shared/i18n/i18n.ts';

export const t = messages({
  fr: {
    search: 'Rechercher par nom',
    choose: 'Choisir',
    cancel: 'Annuler',
    nothing: 'Aucun choix possible.',
    noMatch: 'Aucun résultat pour « {query} ».',
    selected: { zero: 'Aucune sélection', one: '{count} sélectionné', other: '{count} sélectionnés' },
  },
  en: {
    search: 'Search by name',
    choose: 'Choose',
    cancel: 'Cancel',
    nothing: 'Nothing to choose from.',
    noMatch: 'No results for ‘{query}’.',
    selected: { zero: 'None selected', one: '{count} selected', other: '{count} selected' },
  },
});
