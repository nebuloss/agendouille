# The Android app

« Guémené » under its icon, agendouille everywhere else. A sync app: it
puts the family's calendars in the phone's own calendar, so any calendar
app shows and edits the events; it never shows an event itself. Plain
Java around a WebView for its screens; no Google Play services, no Gradle,
no AndroidX.

## Install

- From the server: the sign-in page (`/`) offers it by platform: on an
  Android phone, « Télécharger l'application »; on a computer, a QR code
  to scan with the phone's camera (and a direct link); on an iPhone,
  nothing (`/app/agendouille.apk`).
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
  device on the server).
- **Calendriers**: mine and those shared with me. A calendar's page:
  « Synchroniser sur ce téléphone », its shares (each one's access in
  place; « Partager avec d'autres » picks groups and people), « Modifier »,
  « Transférer », « Supprimer »; for one shared with me, « Personnaliser »
  and « Masquer ».
- **Appareils**: « Connecter un navigateur » (scan a browser's sign-in QR
  code, or type its code, then « Se connecter »), the synced devices and
  the signed-in browsers, each with « Déconnecter ».
- **Réglages**: language, appearance (light, dark, as the phone), version.

## Updates

The app checks when it opens:

- **A release** (from GitHub) checks GitHub's latest release, at most every
  six hours.
- **A dev build** (from `scripts/devdeploy.sh`) checks the server it is
  signed in to.

Either way it downloads the APK itself, checks its SHA-256 (the release's
`SHA256SUMS`, or the server's announced sum), and hands it to Android's
installer, which asks to confirm. Android refuses an APK signed with
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
