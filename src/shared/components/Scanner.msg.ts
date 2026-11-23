import { messages } from '@/shared/i18n/i18n.ts';

export const t = messages({
  fr: {
    title: 'Scanner un QR code',
    starting: 'Ouverture de l’appareil photo…',
    aim: 'Cadrez le QR code affiché sur l’autre écran.',
    cancel: 'Annuler',
    noCamera: 'Cet appareil n’a pas d’appareil photo utilisable.',
    denied: 'L’accès à l’appareil photo a été refusé. Autorisez-le dans les réglages du téléphone, puis réessayez.',
  },
  en: {
    title: 'Scan a QR code',
    starting: 'Opening the camera…',
    aim: 'Point the camera at the QR code on the other screen.',
    cancel: 'Cancel',
    noCamera: 'This device has no camera that can be used.',
    denied: 'Access to the camera was refused. Allow it in the phone’s settings, then try again.',
  },
});
