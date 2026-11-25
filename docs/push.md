# Instant sync (push)

Without push, a phone sees a change made elsewhere at its next periodic
sync (every 15 minutes at best, Android's floor) or when the app is opened.
With push, the server tells the phone within seconds, and the phone syncs.

Nothing goes through Google, and nothing else to install: agendouille is
its own push service.

## On the phone

The agendouille app: Réglages, « Synchronisation instantanée », « Activée »
(off by default). The app then keeps one connection to the server open.

- **The notification.** Android lets an app stay connected only as a
  foreground service, which must show a notification. It is on the lowest
  channel: no icon in the status bar, folded at the bottom of the shade.
  « Masquer la notification » opens Android's setting for it: turn it off
  and it never shows; the connection goes on. The card then says it is
  hidden. Only this notification's category is turned off, not the app's
  others.
- **Battery.** Allow the app in Réglages, Autorisations (and « Démarrage
  automatique » on a Xiaomi phone), or Android cuts the connection while
  the phone sleeps.
- **DAVx5** (4.4 or later) can use it too: the app is a UnifiedPush
  distributor. In DAVx5's account settings, « Push », choose Guémené.
  Réglages then shows « Utilisée aussi par : DAVx5 ».

Messaging apps need no notification because they use Google's push (FCM),
held by Google Play services or microG: their messages go through
Google's servers. agendouille does not, on purpose (DECISIONS).

## How it works

Standard pieces, from the server to the app:

1. **WebDAV-Push** ([draft](https://github.com/bitfireAT/webdav-push)):
   a device subscribes to each calendar over CalDAV with a push endpoint
   and keys; each change sends that endpoint an encrypted message (Web
   Push: RFC 8291 encryption, RFC 8292 VAPID) naming the calendar and its
   sync token, never an event.
2. **The push service** (RFC 8030): agendouille takes Web Push messages at
   `/push/<id>` and keeps them for the device. Its own messages skip the
   network.
3. **The stream**: the app holds `/api/app/push/stream` open (Server-Sent
   Events, with `Last-Event-ID`); a message waits on the server until the
   app acknowledges it, so a cut connection loses nothing.
4. **UnifiedPush**: the app hands each message on, to itself (a sync) or to
   DAVx5.

A change is sent once its burst is over (300 ms), never to the device that
made it, nor to the registrations its request names in `Push-Dont-Notify`.

## Another push server

WebDAV-Push is standard: DAVx5 can also use any UnifiedPush distributor
(ntfy, NextPush, …) with its own server. agendouille sends to any `https`
endpoint.
