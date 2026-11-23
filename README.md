# agendouille

> *agendouille*: an *agenda* with the *-ouille* of *andouille*. The
> family's calendar, not a serious one.

One shared calendar for a family, self-hosted. Every member reads it and
adds, changes and deletes events, from:

- **Android**, through [DAVx5](https://www.davx5.com/): events appear
  read-write in any calendar app. Pair a phone by scanning a QR code.
- **iPhone and macOS**, natively, with a one-tap configuration profile.
- **Thunderbird** and other CalDAV clients.
- **A web UI**, good on a phone.
- **Anything that reads an `.ics` link** (a TV, Google Calendar), read-only.

One Go binary with the web UI embedded, SQLite, meant to run at home behind
a reverse proxy with TLS.

**Status: planning.** See `PLAN.md` for the design, `CONTRACTS.md` for the
interfaces and `DECISIONS.md` for what was decided and why.
