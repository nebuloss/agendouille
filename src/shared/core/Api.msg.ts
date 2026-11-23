import { messages } from '@/shared/i18n/i18n.ts';

export const t = messages({
  fr: {
    offline: 'Impossible de joindre le serveur. Vérifiez votre connexion, puis réessayez.',
    httpError: 'Une erreur est survenue (code {status}). Réessayez dans quelques instants.',
  },
  en: {
    offline: 'The server cannot be reached. Check your connection and try again.',
    httpError: 'Something went wrong (code {status}). Please try again in a moment.',
  },
});
