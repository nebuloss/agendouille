package calendar

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nebuloss/agendouille/internal/directory"
	"github.com/nebuloss/agendouille/internal/hub"
	"github.com/nebuloss/agendouille/internal/settings"
	"github.com/nebuloss/agendouille/internal/sharing"
	"github.com/nebuloss/agendouille/internal/store"
)

type fx struct {
	cal               *Service
	dir               *directory.Service
	now               *time.Time
	maman, lea, mamie directory.User
	set               *settings.Service
}

func setup(t *testing.T) *fx {
	t.Helper()
	ctx := context.Background()
	st, err := store.OpenDir(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	now := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	set, _ := settings.Open(ctx, st.DB(), clock)
	dir := directory.New(st.DB(), clock, func() []string { return settings.DefaultPalette })
	dir.AddGroup(ctx, "t", "famille", "", "")
	dir.AddGroup(ctx, "t", "enfants", "", "")
	maman, _ := dir.AddUser(ctx, "t", "maman", "", "", []string{"famille"})
	lea, _ := dir.AddUser(ctx, "t", "lea", "", "", []string{"famille", "enfants"})
	mamie, _ := dir.AddUser(ctx, "t", "mamie", "", "", nil)
	f := &fx{cal: New(st.DB(), clock, set, dir, sharing.New(st.DB(), dir), hub.New()), dir: dir, now: &now,
		maman: maman, lea: lea, mamie: mamie, set: set}
	return f
}

func as(u directory.User) Actor {
	return Actor{UserID: u.ID, Name: u.Name, Cap: "readwrite", Via: "test"}
}

func ev(uid, summary string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:test\r\nBEGIN:VEVENT\r\nUID:" + uid + "\r\nSUMMARY:" + summary +
		"\r\nDTSTART;TZID=Europe/Paris:20261125T140000\r\nDTEND;TZID=Europe/Paris:20261125T153000\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
}

func TestOwnerSharesAndAccess(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	name := "École"
	ecole, err := f.cal.Create(ctx, as(f.maman), "maman", "Ecole_2026", Props{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if ecole.Ref.String() != "maman~ecole-2026" || ecole.Access != sharing.Owner {
		t.Fatalf("created %+v", ecole)
	}
	r := ecole.Ref
	if _, _, err := f.cal.Get(ctx, as(f.lea), r, "x.ics"); !errors.Is(err, ErrNotFound) {
		t.Errorf("before any share, lea: %v", err)
	}
	sh := f.cal.Sharing()
	at := store.Time(*f.now)
	if err := sh.ShareWith(ctx, "maman", f.maman.ID, true, ecole.ID, sharing.Target{User: "mamie"}, sharing.Read, at); !errors.Is(err, sharing.ErrNotVisible) {
		t.Errorf("sharing with someone maman does not see: %v", err)
	}
	if err := sh.ShareWith(ctx, "maman", f.maman.ID, true, ecole.ID, sharing.Target{Group: "enfants"}, sharing.Read, at); !errors.Is(err, sharing.ErrNotVisible) {
		t.Errorf("sharing with a group maman is not in: %v", err)
	}
	if err := sh.ShareWith(ctx, "maman", f.maman.ID, true, ecole.ID, sharing.Target{Group: "famille"}, sharing.Read, at); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.cal.Put(ctx, as(f.lea), r, "a.ics", ev("a", "Rentrée"), Precondition{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("a read share writing: %v", err)
	}
	// A user share and a group share: the higher wins.
	if err := sh.ShareWith(ctx, "maman", f.maman.ID, true, ecole.ID, sharing.Target{User: "lea"}, sharing.ReadWrite, at); err != nil {
		t.Fatal(err)
	}
	if _, created, err := f.cal.Put(ctx, as(f.lea), r, "a.ics", ev("a", "Rentrée"), Precondition{IfNoneMatch: true}); err != nil || !created {
		t.Fatalf("lea writing with readwrite: %v", err)
	}
	// A read-only device never writes, whatever the share.
	ro := as(f.lea)
	ro.Cap = "read"
	if _, _, err := f.cal.Put(ctx, ro, r, "b.ics", ev("b", "x"), Precondition{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("a read device: %v", err)
	}
	home, _ := f.cal.Home(ctx, as(f.lea), false)
	if len(home) != 1 || home[0].Ref != r || home[0].Access != sharing.ReadWrite {
		t.Fatalf("lea's home: %+v", home)
	}
	// Hiding takes it off lea's home only.
	yes := true
	sh.SetView(ctx, f.lea.ID, ecole.ID, nil, &yes, nil, nil, nil)
	if home, _ := f.cal.Home(ctx, as(f.lea), false); len(home) != 0 {
		t.Errorf("a hidden calendar is in the home: %+v", home)
	}
	if _, _, err := f.cal.Get(ctx, as(f.lea), r, "a.ics"); err != nil {
		t.Errorf("a hidden calendar is still readable by URL: %v", err)
	}
	// Leaving the group and losing the user share: gone.
	sh.Unshare(ctx, "maman", ecole.ID, sharing.Target{User: "lea"}, at)
	f.dir.RemoveMember(ctx, "t", "famille", "lea")
	if _, _, err := f.cal.Get(ctx, as(f.lea), r, "a.ics"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after losing every share: %v", err)
	}
}

func TestPutPreconditionsAndUIDs(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c, _ := f.cal.Create(ctx, as(f.lea), "lea", "perso", Props{})
	r := c.Ref
	o, _, err := f.cal.Put(ctx, as(f.lea), r, "a.ics", ev("a", "one"), Precondition{IfNoneMatch: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.cal.Put(ctx, as(f.lea), r, "a.ics", ev("a", "two"), Precondition{IfNoneMatch: true}); !errors.Is(err, ErrPrecondition) {
		t.Errorf("If-None-Match * on an existing object: %v", err)
	}
	if _, _, err := f.cal.Put(ctx, as(f.lea), r, "a.ics", ev("a", "two"), Precondition{IfMatch: []string{`"stale"`}}); !errors.Is(err, ErrPrecondition) {
		t.Errorf("a stale If-Match: %v", err)
	}
	o2, created, err := f.cal.Put(ctx, as(f.lea), r, "a.ics", ev("a", "two"), Precondition{IfMatch: []string{o.ETag}})
	if err != nil || created || o2.ETag == o.ETag {
		t.Fatalf("an update with the right etag: %v %v", err, created)
	}
	var conflict *UIDConflict
	if _, _, err := f.cal.Put(ctx, as(f.lea), r, "b.ics", ev("a", "dup"), Precondition{}); !errors.As(err, &conflict) || conflict.Name != "a.ics" {
		t.Errorf("the same UID under another name: %v", err)
	}
	if _, _, err := f.cal.Put(ctx, as(f.lea), r, "../x.ics", ev("z", ""), Precondition{}); !errors.Is(err, ErrBadName) {
		t.Errorf("a bad name: %v", err)
	}
	if _, _, err := f.cal.Put(ctx, as(f.lea), r, "t.ics", []byte("BEGIN:VCALENDAR\r\nBEGIN:VTODO\r\nUID:t\r\nEND:VTODO\r\nEND:VCALENDAR\r\n"), Precondition{}); !errors.Is(err, ErrUnsupportedComp) {
		t.Errorf("a task: %v", err)
	}
	_, got, _ := f.cal.Get(ctx, as(f.lea), r, "a.ics")
	if string(got.Data) != string(ev("a", "two")) {
		t.Error("the stored bytes are not what was put")
	}
	if err := f.cal.Delete(ctx, as(f.lea), r, "a.ics", Precondition{IfMatch: []string{o.ETag}}); !errors.Is(err, ErrPrecondition) {
		t.Errorf("a delete with a stale etag: %v", err)
	}
	if err := f.cal.Delete(ctx, as(f.lea), r, "a.ics", Precondition{IfMatch: []string{o2.ETag}}); err != nil {
		t.Fatal(err)
	}
	if err := f.cal.Delete(ctx, as(f.lea), r, "a.ics", Precondition{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting twice: %v", err)
	}
}

func TestChangesSinceAndPruning(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c, _ := f.cal.Create(ctx, as(f.lea), "lea", "perso", Props{})
	r := c.Ref
	f.cal.Put(ctx, as(f.lea), r, "a.ics", ev("a", "a"), Precondition{})
	f.cal.Put(ctx, as(f.lea), r, "b.ics", ev("b", "b"), Precondition{})
	i, _ := f.cal.Info(ctx, as(f.lea), r)
	first, err := f.cal.ChangesSince(ctx, i, 0, 0)
	if err != nil || strings.Join(first.Names, ",") != "a.ics,b.ics" {
		t.Fatalf("a first sync: %+v %v", first, err)
	}
	f.cal.Put(ctx, as(f.lea), r, "a.ics", ev("a", "a2"), Precondition{})
	f.cal.Delete(ctx, as(f.lea), r, "b.ics", Precondition{})
	f.cal.Put(ctx, as(f.lea), r, "c.ics", ev("c", "c"), Precondition{})
	i, _ = f.cal.Info(ctx, as(f.lea), r)
	next, err := f.cal.ChangesSince(ctx, i, first.Seq, 0)
	if err != nil || strings.Join(next.Names, ",") != "a.ics,b.ics,c.ics" || next.Seq != i.Seq {
		t.Fatalf("since the first token: %+v %v", next, err)
	}
	limited, _ := f.cal.ChangesSince(ctx, i, first.Seq, 2)
	if !limited.Truncated || len(limited.Names) != 2 {
		t.Fatalf("with a limit of 2: %+v", limited)
	}
	rest, _ := f.cal.ChangesSince(ctx, i, limited.Seq, 0)
	if strings.Join(rest.Names, ",") != "c.ics" {
		t.Errorf("after the truncated page: %+v", rest)
	}
	if _, err := f.cal.ChangesSince(ctx, i, i.Seq+5, 0); !errors.Is(err, ErrTokenExpired) {
		t.Errorf("a token from the future: %v", err)
	}
	// 181 days later, the change log is pruned: old tokens are refused.
	*f.now = f.now.AddDate(0, 0, 181)
	if _, err := f.cal.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	i, _ = f.cal.Info(ctx, as(f.lea), r)
	if _, err := f.cal.ChangesSince(ctx, i, first.Seq, 0); !errors.Is(err, ErrTokenExpired) {
		t.Errorf("a pruned token: %v", err)
	}
	if ch, err := f.cal.ChangesSince(ctx, i, i.Seq, 0); err != nil || len(ch.Names) != 0 {
		t.Errorf("the current token after pruning: %+v %v", ch, err)
	}
	tok := Token("https://agenda.example.org", i)
	if n, err := ParseToken("https://agenda.example.org/", i, tok); err != nil || n != i.Seq {
		t.Errorf("token round trip: %d %v", n, err)
	}
	if _, err := ParseToken("https://agenda.example.org", i, "https://evil/ns/sync/1/2"); !errors.Is(err, ErrTokenExpired) {
		t.Errorf("a foreign token: %v", err)
	}
}

func TestRangeUsesRecurrence(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c, _ := f.cal.Create(ctx, as(f.lea), "lea", "perso", Props{})
	weekly := []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:t\r\nBEGIN:VEVENT\r\nUID:judo\r\n" +
		"DTSTART;TZID=Europe/Paris:20261104T140000\r\nDTEND;TZID=Europe/Paris:20261104T153000\r\n" +
		"RRULE:FREQ=WEEKLY\r\nEXDATE;TZID=Europe/Paris:20261118T140000\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	f.cal.Put(ctx, as(f.lea), c.Ref, "judo.ics", weekly, Precondition{})
	f.cal.Put(ctx, as(f.lea), c.Ref, "once.ics", ev("once", "x"), Precondition{}) // 25 Nov
	i, _ := f.cal.Info(ctx, as(f.lea), c.Ref)
	day := func(d int) (time.Time, time.Time) {
		s := time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC)
		return s, s.Add(24 * time.Hour)
	}
	count := func(d int) int {
		from, to := day(d)
		objs, err := f.cal.Range(ctx, i, from, to)
		if err != nil {
			t.Fatal(err)
		}
		return len(objs)
	}
	if n := count(11); n != 1 {
		t.Errorf("11 Nov (a Wednesday): %d objects, want judo", n)
	}
	if n := count(18); n != 0 {
		t.Errorf("18 Nov (excluded): %d", n)
	}
	if n := count(12); n != 0 {
		t.Errorf("12 Nov (a Thursday): %d", n)
	}
	if n := count(25); n != 2 {
		t.Errorf("25 Nov: %d, want judo and the one-off", n)
	}
}

func TestQuotasImportExportTransfer(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.set.Set(ctx, "t", map[string]string{"max_calendars_per_user": "1", "max_events_per_calendar": "2"})
	c, _ := f.cal.Create(ctx, as(f.lea), "lea", "perso", Props{})
	if _, err := f.cal.Create(ctx, as(f.lea), "lea", "second", Props{}); !errors.Is(err, ErrQuota) {
		t.Errorf("a second calendar over the limit: %v", err)
	}
	if _, err := f.cal.Create(ctx, as(f.lea), "maman", "x", Props{}); !errors.Is(err, ErrNotOwner) {
		t.Errorf("creating for someone else: %v", err)
	}
	file := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:google\r\n" +
		"BEGIN:VTIMEZONE\r\nTZID:Europe/Paris\r\nBEGIN:STANDARD\r\nDTSTART:19701025T030000\r\nTZOFFSETFROM:+0200\r\nTZOFFSETTO:+0100\r\nEND:STANDARD\r\nEND:VTIMEZONE\r\n" +
		"BEGIN:VEVENT\r\nUID:one@google.com\r\nSUMMARY:Un\r\nDTSTART;TZID=Europe/Paris:20261201T100000\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:two@google.com\r\nSUMMARY:Deux\r\nDTSTART:20261202T100000Z\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:three@google.com\r\nSUMMARY:Trois\r\nDTSTART:20261203T100000Z\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	res, err := f.cal.Import(ctx, Operator("cli"), c.Ref, []byte(file), false)
	if err != nil || res.Added != 2 || len(res.Errors) != 1 {
		t.Fatalf("import over the event limit: %+v %v", res, err)
	}
	f.set.Set(ctx, "t", map[string]string{"max_events_per_calendar": "100"})
	res, _ = f.cal.Import(ctx, Operator("cli"), c.Ref, []byte(file), false)
	if res.Added != 1 || res.Skipped != 2 {
		t.Errorf("import again: %+v", res)
	}
	_, one, err := f.cal.Get(ctx, as(f.lea), c.Ref, "one@google.com.ics")
	if err != nil || !strings.Contains(string(one.Data), "BEGIN:VTIMEZONE") || one.CreatedBy != f.lea.ID {
		t.Errorf("an imported object keeps its zone and is the owner's: %v %s", err, one.Data)
	}
	out, err := f.cal.Export(ctx, as(f.lea), c.Ref)
	if err != nil || strings.Count(string(out), "BEGIN:VEVENT") != 3 || strings.Count(string(out), "BEGIN:VTIMEZONE") != 1 {
		t.Errorf("export: %v\n%s", err, out)
	}
	moved, err := f.cal.Transfer(ctx, Operator("cli"), c.Ref, "maman", true)
	if err != nil || moved.Ref.String() != "maman~perso" {
		t.Fatalf("transfer: %+v %v", moved, err)
	}
	if acc, _ := f.cal.Sharing().Access(ctx, f.lea.ID, moved.ID); acc != sharing.ReadWrite {
		t.Errorf("the old owner keeps %v, want readwrite", acc)
	}
	if err := f.cal.DeleteCalendar(ctx, as(f.lea), moved.Ref); !errors.Is(err, ErrNotOwner) {
		t.Errorf("a sharee deleting: %v", err)
	}
	if err := f.cal.DeleteCalendar(ctx, as(f.maman), moved.Ref); err != nil {
		t.Fatal(err)
	}
	if _, err := f.cal.Create(ctx, as(f.maman), "maman", "perso", Props{}); !errors.Is(err, ErrExists) {
		t.Errorf("a deleted calendar's slug was reused: %v", err)
	}
}
