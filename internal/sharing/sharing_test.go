package sharing

import (
	"context"
	"testing"
	"time"

	"github.com/nebuloss/agendouille/internal/directory"
	"github.com/nebuloss/agendouille/internal/settings"
	"github.com/nebuloss/agendouille/internal/store"
)

func TestAccessRules(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenDir(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	dir := directory.New(st.DB(), time.Now, func() []string { return settings.DefaultPalette })
	dir.AddGroup(ctx, "t", "famille", "", "")
	dir.AddGroup(ctx, "t", "enfants", "", "")
	owner, _ := dir.AddUser(ctx, "t", "maman", "", "", []string{"famille"})
	kid, _ := dir.AddUser(ctx, "t", "lea", "", "", []string{"famille", "enfants"})
	other, _ := dir.AddUser(ctx, "t", "voisin", "", "", nil)
	res, _ := st.DB().Exec(`INSERT INTO calendars(owner_id, slug, name, color, timezone, created_at) VALUES (?, 'ecole', 'École', '#000000', 'UTC', '2026')`, owner.ID)
	cal, _ := res.LastInsertId()
	s := New(st.DB(), dir)
	at := "2026-11-23T00:00:00.000000000Z"

	check := func(u directory.User, want Access) {
		t.Helper()
		if got, err := s.Access(ctx, u.ID, cal); err != nil || got != want {
			t.Errorf("%s: %v, want %v (%v)", u.Name, got, want, err)
		}
	}
	check(owner, Owner)
	check(kid, None)
	s.ShareWith(ctx, "maman", owner.ID, true, cal, Target{Group: "famille"}, Read, at)
	check(kid, Read)
	s.ShareWith(ctx, "maman", owner.ID, true, cal, Target{User: "lea"}, ReadWrite, at)
	check(kid, ReadWrite) // the higher of the user share and the group share
	s.ShareWith(ctx, "maman", owner.ID, true, cal, Target{User: "lea"}, Read, at)
	check(kid, Read) // changing a share replaces it
	check(other, None)
	if err := s.ShareWith(ctx, "maman", owner.ID, true, cal, Target{User: "voisin"}, Read, at); err != ErrNotVisible {
		t.Errorf("sharing with a stranger: %v", err)
	}
	if err := s.ShareWith(ctx, "admin", owner.ID, false, cal, Target{User: "voisin"}, Read, at); err != nil {
		t.Errorf("the operator sharing with anyone: %v", err)
	}
	check(other, Read)
	if Effective(Owner, "read") != Read || Effective(ReadWrite, "readwrite") != ReadWrite {
		t.Error("Effective")
	}
	shares, _ := s.Shares(ctx, cal)
	if len(shares) != 3 || shares[0].Name != "u:lea" {
		t.Errorf("shares %+v", shares)
	}
	// Deleting the calendar takes every access away.
	st.DB().Exec(`UPDATE calendars SET deleted_at = 'x' WHERE id = ?`, cal)
	check(owner, None)
	check(kid, None)
	for _, bad := range []string{"lea", "x:lea", "u:", "g:"} {
		if _, err := ParseTarget(bad); err == nil {
			t.Errorf("target %q was accepted", bad)
		}
	}
}
