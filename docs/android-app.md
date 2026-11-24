# The Android app

« Guémené » under its icon, agendouille everywhere else. A sync app: it
puts the family's calendars in the phone's own calendar, so any calendar
app shows and edits the events; it never shows an event itself. Plain
Java around a WebView for its screens; no Google Play services, no Gradle,
no AndroidX.

## Install

- From the server: on an Android phone, the sign-in page (`/`) offers
  only « Télécharger l'application » (`/app/agendouille.apk`), no sign-in
  code: a phone signs in with the app. Another phone (an iPhone) gets the
  short code to type in the app of a signed-in phone; only a computer
  shows the QR code.
- From GitHub: `agendouille-<version>.apk` on each
  [release](https://github.com/nebuloss/agendouille/releases), with its
  line in `SHA256SUMS`.

Android asks once to allow installs from the browser (or the app, for its
updates). Android 8 or later; phones without Google Play services work.

## Sign in

The welcome screen offers:

- **Scanner le QR code**: the admin's sign-in QR code (the full app), or
  a DAVx5 device's QR code (sync only).
- **Configurer à la main**: the server, a user name and a device password
  (from Appareils, « Autre »), checked over CalDAV.

A phone signed in with a device password (by hand, or a DAVx5 QR code)
syncs only: managing calendars and devices needs the app's own sign-in,
which opens a session for the person.

## The tabs

- **Synchro**: the last sync, « Synchroniser » now, how many events and
  calendars are on the phone, the account, « Se déconnecter » (removes the
  account and its calendars from the phone; an app sign-in also removes the
  device on the server). While a permission is missing (battery
  optimisation included), a small « N autorisations à accorder » link opens
  Réglages' Autorisations.
- **Calendriers**: mine and those shared with me. A calendar's page:
  « Synchroniser sur ce téléphone », its shares (each one's access in
  place; « Partager avec d'autres » picks groups and people), « Modifier »,
  « Transférer », « Supprimer »; for one shared with me, « Personnaliser »
  and « Masquer ».
- **Appareils**: « Connecter un navigateur » (scan a browser's sign-in QR
  code, or type its code, then « Se connecter »), the synced devices and
  the signed-in browsers, each with « Déconnecter », and « Déconnecter tous
  les navigateurs » (this phone stays signed in).
- **Réglages**: « Autorisations » (each permission, granted or not, with
  its button: calendar, notifications, camera, battery and, on a Xiaomi
  phone, autostart), language, appearance (light, dark, as the phone),
  version, « Versions de développement », and « Rechercher une mise à
  jour ».

## Background sync

The app syncs through Android's sync framework, as DAVx5 does: every
`app_sync_interval_minutes` (the server's setting, 15 minutes at least),
when an event is changed on the phone, and when the app is opened (if the
last sync is over five minutes old). There is no push yet: a change made
elsewhere arrives at the next sync.

Android holds back the background work of apps under battery optimisation,
sometimes for hours on a phone left idle. Allow the app in Réglages,
Autorisations (Synchro links there while one is missing). On Xiaomi phones (MIUI, Redmi, POCO), also turn on
« Démarrage automatique » for agendouille, or it only syncs while open:
Android cannot tell the app whether it is on, so the app only opens the
setting.

## Updates

The app checks when it opens, and when « Rechercher une mise à jour »
(Réglages) is tapped, which also ignores an earlier « Plus tard ». Where it
looks is the switch « Versions de développement » (Réglages):

- **Off** (a release's default): GitHub's latest release, at most every six
  hours.
- **On** (a dev build's default): the server it is signed in to, which
  offers the build it embeds (`scripts/devdeploy.sh`), whatever its version.

Android never installs a lower version code: the app checks the APK's
before handing it over and says so, and a dev build switched off is offered
a release only once one is later than it.

Either way it downloads the APK itself, checks its SHA-256 (the release's
`SHA256SUMS`, or the server's announced sum), and hands it to Android's
installer, which asks to confirm. Only one APK is ever kept, in the app's
cache, while it is being installed: it is deleted once Android has its
copy, and an install that stopped halfway is cleaned up at the next start.
Old versions are never stored. Android refuses an APK signed with
another key: every build is signed with the release key when the build
machine has it, so releases and dev builds install over each other, the
higher version code (the commit count) winning.

## When it stops

If the app crashes, it keeps a report; the next start shows it with
« Envoyer au serveur » (written to the server's log), « Copier » and
« Ignorer ».

## Under the hood

- `android/src/…`: `MainActivity` (the screens, the bridge to them),
  `Credentials` (the account, its secret in the Keystore),
  `SyncService` with `CalendarSync`, `EventSync`, `UploadSync` (CalDAV both
  ways, as DAVx5 does it), `ICal`, `EventMapper`, `EventWriter` (it patches
  the iCalendar text, never regenerates it), `Updater`, `CrashLog`.
- `src/android/`: the screens, TypeScript, built into `android/assets/web`.
  The Calendriers tab reuses the web app's code over the bridge; Java only
  lets through the paths it needs (`ApiPath`: never events).
- Building and testing: [development](development.md#the-android-app).
