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
- **Every other browser** shows a QR code, a short code and a number on
  the sign-in page (`/`). A device already signed in approves it: the
  agendouille app's Appareils tab (scan, or type the code), or `/approve`
  in a signed-in browser. The approver taps the number the browser shows;
  a wrong one cancels the request.
- **Another phone** with the agendouille app: Appareils, « agendouille »
  shows the app's own sign-in code (ten minutes, one use).

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

## Device keys

Each synced device has its own credential, revoked on its own (Appareils,
« Supprimer »; or the admin UI). The admin chooses how CalDAV devices sign
in (admin UI, Réglages, « Connexion des appareils »), and can set it per
user:

| Mode | What a device presents | Clients |
|---|---|---|
| « Mot de passe » | its password | all |
| « Mot de passe et certificat » | both: a stolen password is not enough | DAVx5, Thunderbird; the iPhone not always |
| « Certificat seul » | a client certificate (`.p12`) | DAVx5, Thunderbird |

agendouille is its own certificate authority; a device's `.p12` is shown
once, at creation (or « Certificat » on its row makes a new one). The DMZ
proxy has to ask for it and pass it on: [deployment](deployment.md#device-certificates).

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
