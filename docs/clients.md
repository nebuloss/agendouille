# Clients: phones, computers, browsers

Sync is standard CalDAV for everyone, the agendouille app included: any
CalDAV client works. Each person sets up their own devices from the family
web app's **Appareils** page, which writes everything the client needs.

## Signing in, without passwords

Nobody has a password or an email address.

- **The first device** is signed in by a QR code the admin makes (the
  user's Appareils page in the admin UI, « Créer un QR code »). It works
  once, for 72 hours (`enrollment_link_ttl_hours`). Scanned with the
  agendouille app, it signs the app in; opened in a browser, it signs that
  browser in.
- **Every other browser** shows a QR code and a short code on the sign-in
  page (`/`). A device already signed in scans it (the agendouille app's
  Appareils tab) or types the code (there, or at `/approve` in a signed-in
  browser); the browser then says whose phone to look at. The phone shows
  which browser asks and from where, and « Se connecter » or « Ce n'est
  pas moi », as WhatsApp's and Discord's QR sign-in. Never confirm a code
  someone sent you: the browser that gets in is theirs.
- **Another phone** with the agendouille app: a new QR code from the
  admin, as for the first. A browser cannot make one: it was approved by
  a device, and would hand out more than it has.

## Android: the agendouille app

The recommended client: it syncs into the phone's own calendar, manages
calendars and signs browsers in. See [the Android app](android-app.md).

## Android: DAVx5

Appareils, « Android (DAVx5) »: the dialog shows a QR code. Scan it with
the phone's camera: it opens a page of ours with « Ouvrir dans DAVx5 »,
which fills the account in (DAVx5 has no scanner, and a camera app cannot
open `davx5://`). The same page lists the address, user and password for
setting it up by hand (in DAVx5, « Se connecter avec une URL et un nom
d'utilisateur »). The page works for
ten minutes. The agendouille app can scan the same QR code too.

## iPhone, iPad, Mac

Appareils, « iPhone, iPad, Mac »: the dialog offers a configuration
profile; open it on the device, then Settings, « Profile Downloaded »,
Install. By hand: Settings, Calendar, Accounts, Add Account, Other, Add
CalDAV Account, with the server, user name and password the dialog shows.

## Thunderbird

Appareils, « Thunderbird »: New Calendar, On the Network, with the user
name and the address the dialog shows, then the password when asked.

## Anything else

« Autre » gives the server address (`https://<public_url>/dav/`), a user
name and a password. A calendar made in a client belongs to its user and
shows everywhere.

## Read-only links

Appareils, « Liens d'abonnement »: an `https://…/ics/…` address that any
calendar application can subscribe to, for one or more calendars, read
only. Removing the link stops it at once.

## Signing browsers out

Appareils lists the browsers signed in, each with « Déconnecter »; with
several, « Déconnecter les autres » (web app) or « Déconnecter tous les
navigateurs » (the Android app) signs them all out at once, the one asking
and the Android app staying signed in. The admin has « Tout déconnecter »
on a user's Appareils page. A browser signed out has to be approved again.

## Device passwords

Each synced device has its own password, made by the server (long and
random, never typed: it reaches the device by a QR code, a link, a profile
or the app's sign-in), shown once, and revoked on its own (Appareils,
« Supprimer », or the admin UI) without touching the others. Appareils
shows when each one last synced. That is the only way a CalDAV client
signs in; client certificates were removed (DECISIONS 2026-11-24).

## Checked by hand

After an upgrade that touches CalDAV, on a test calendar:

| Step | Web app | agendouille app | DAVx5 | iPhone | Thunderbird |
|---|---|---|---|---|---|
| Set up from Appareils | QR or link | QR | QR, one tap | profile | by hand |
| The calendars appear, with their colours | | | | | |
| An event made here shows on the others | | | | | |
| A weekly event, one Wednesday moved | | | | | |
| An alarm made on the phone survives a web edit | | | | | |
| A calendar shared read-only cannot be written | | | | | |
| A hidden calendar leaves the phone | | | | | |
| A revoked device stops syncing | | | | | |
