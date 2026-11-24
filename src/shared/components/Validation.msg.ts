import { messages } from '@/shared/i18n/i18n.ts';

export const t = messages({
  fr: {
    required: 'Ce champ est obligatoire.',
    check: 'Cochez cette case pour continuer.',
    email: 'Saisissez une adresse e-mail valide.',
    url: 'Saisissez une adresse complète, par exemple https://agenda.example.org.',
    format: 'Le format n’est pas valide.',
    tooShort: { one: 'Saisissez au moins {n} caractère.', other: 'Saisissez au moins {n} caractères.' },
    tooLong: { one: 'Saisissez au plus {n} caractère.', other: 'Saisissez au plus {n} caractères.' },
    min: 'La valeur doit être d’au moins {n}.',
    max: 'La valeur doit être d’au plus {n}.',
  },
  en: {
    required: 'This field is required.',
    check: 'Tick this box to continue.',
    email: 'Enter a valid e-mail address.',
    url: 'Enter a full address, for example https://agenda.example.org.',
    format: 'This is not in a valid format.',
    tooShort: { one: 'Enter at least {n} character.', other: 'Enter at least {n} characters.' },
    tooLong: { one: 'Enter at most {n} character.', other: 'Enter at most {n} characters.' },
    min: 'The value must be at least {n}.',
    max: 'The value must be at most {n}.',
  },
});
