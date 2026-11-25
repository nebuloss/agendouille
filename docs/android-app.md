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

## The sync mode

Once, after signing in, the app asks how it syncs: « Standard » (every 15
minutes, on opening, after a change on the phone) or « Instantanée » (a
change made elsewhere within seconds: see [instant sync](push.md)). Réglages,
« Mode de synchronisation », changes it. Each mode asks only for what it
needs: the battery exemption only for instant sync; the camera when
scanning.

## The tabs

- **Synchro**: the last sync, « Synchroniser » now, how many events and
  calendars are on the phone, the account, « Se déconnecter » (removes the
  account and its calendars from the phone; an app sign-in also removes the
  device on the server). While a permission is missing (battery
  optimisation included), a small « N autorisations à accorder » link opens
  Réglages' Autorisations.
- **Calendriers**: mine and those shared with me, each with its
  « Affiché » switch. The switch is the person's, kept on the server: the
  web agenda follows it, and so does every phone with the app at its next
  sync (a calendar turned off leaves the phone's agenda). A calendar's page:
  « Synchroniser sur ce téléphone », its shares (each one's access in
  place; « Partager avec d'autres » picks groups and people), « Modifier »,
  « Transférer », « Supprimer »; for one shared with me, « Personnaliser »
  and « Masquer ».
- **Appareils**: « Connecter un navigateur » (scan a browser's sign-in QR
  code, or type its code, then « Se connecter »), the synced devices and
  the signed-in browsers, each with « Déconnecter », and « Déconnecter tous
  les navigateurs » (this phone stays signed in).
- **Réglages**: « Autorisations » (each permission, granted or not, with
  its button: calendar, notifications, camera, battery (needed by instant
  sync), instant sync's notification while it is on, and, under MIUI or HyperOS
  phone, autostart), « Synchronisation instantanée » (push, off by
  default: on, the app stays connected, with a discreet notification it
  can hide), language, appearance (light, dark, as the phone),
  version, « Versions de développement », and « Rechercher une mise à
  jour ».

## Background sync

The app syncs through Android's sync framework, as DAVx5 does: every
`app_sync_interval_minutes` (the server's setting, 15 minutes at least),
when the app is opened, and within seconds of an event being created,
changed or deleted on the phone (in any calendar app, the app closed or
not: a job Android runs when the phone's calendars change). A change made
elsewhere arrives within seconds with instant sync (Réglages, « Synchronisation
instantanée », see [instant sync](push.md)), at the next sync without it.

Android holds back the background work of apps under battery optimisation,
sometimes for hours on a phone left idle. Allow the app in Réglages,
Autorisations (Synchro links there while one is missing). Under MIUI or HyperOS (Xiaomi's systems, not crDroid or LineageOS on a Xiaomi phone), also turn on
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

It shows a dialog (the version now and the new one, where it comes from); « Mettre à jour » shows the download, then Android's own confirmation, or the error, in the same dialog. Either way it downloads the APK itself, checks its SHA-256 (the release's
`SHA256SUMS`, or the server's announced sum), and hands it to Android's
installer, which asks to confirm. Only one APK is ever kept, in the app's
cache, while it is being installed: it is deleted once Android has its
copy, and an install that stopped halfway is cleaned up at the next start.
Old versions are never stored. Android refuses an APK signed with
another key: every build is signed with the release key when the build
machine has it, so releases and dev builds install over each other, the
higher version code (the commit count) winning.

## Alerts

With the notifications permission, the app tells, in its « Alertes »
category: when the server no longer accepts the phone (sign in again), when
sync fails twice in a row while the phone is online (never when offline),
and when the sync mode lacks a permission (the calendar; the battery for
instant sync), once for each new lack; tapping that one opens the
permissions. Each goes away once things are right again.

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
