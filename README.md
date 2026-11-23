# agendouille

> *agendouille*: an *agenda* with the *-ouille* of *andouille*. The
> family's calendar, not a serious one.

Shared calendars for a family, self-hosted. Every member reads them and
adds, changes and deletes events (as each calendar allows), from:

- **Android**, through [DAVx5](https://www.davx5.com/): events appear
  read-write in any calendar app. Pair a phone by scanning a QR code.
- **iPhone and macOS**, natively, with a one-tap configuration profile.
- **Thunderbird** and other CalDAV clients.
- **A web app**, good on a phone. Admins have a separate admin UI.
- **Anything that reads an `.ics` link** (a TV, Google Calendar), read-only.

One Go binary with the family app and the admin UI embedded, SQLite. Meant
to run at home: CalDAV and the family app behind a reverse proxy in the
DMZ, the admin UI on the LAN.

**Status: planning.** See `PLAN.md` for the design, `CONTRACTS.md` for the
interfaces and `DECISIONS.md` for what was decided and why.
