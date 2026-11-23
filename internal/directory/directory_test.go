package directory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nebuloss/agendouille/internal/settings"
	"github.com/nebuloss/agendouille/internal/store"
)

func newDir(t *testing.T) *Service {
	t.Helper()
	st, err := store.OpenDir(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st.DB(), time.Now, func() []string { return settings.DefaultPalette })
}

func names(us []User) []string {
	out := []string{}
	for _, u := range us {
		out = append(out, u.Name)
	}
	return out
}

func TestUsersGroupsAndVisibility(t *testing.T) {
	d := newDir(t)
	ctx := context.Background()
	for _, g := range []string{"famille", "grands-parents"} {
		if _, err := d.AddGroup(ctx, "admin", g, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	maman, _ := d.AddUser(ctx, "admin", "maman", "Maman", "", []string{"famille", "grands-parents"})
	lea, _ := d.AddUser(ctx, "admin", "lea", "Léa", "", []string{"famille"})
	mamie, err := d.AddUser(ctx, "admin", "mamie", "", "", []string{"grands-parents"})
	if err != nil {
		t.Fatal(err)
	}
	if mamie.DisplayName != "Mamie" || maman.Color == lea.Color {
		t.Errorf("defaults: %+v %+v %+v", mamie, maman, lea)
	}

	check := func(a, b User, want bool) {
		t.Helper()
		got, err := d.Sees(ctx, a.ID, b.ID)
		if err != nil || got != want {
			t.Errorf("%s sees %s: %v, want %v (%v)", a.Name, b.Name, got, want, err)
		}
	}
	check(maman, lea, true)
	check(maman, mamie, true)
	check(lea, mamie, false) // no group in common
	check(lea, lea, true)

	vis, _ := d.Visible(ctx, maman.ID)
	if got := names(vis); len(got) != 2 || got[0] != "lea" || got[1] != "mamie" {
		t.Errorf("maman sees %v", got)
	}

	// Leaving a group ends the visibility it gave.
	if err := d.RemoveMember(ctx, "admin", "grands-parents", "mamie"); err != nil {
		t.Fatal(err)
	}
	check(maman, mamie, false)
}

func TestNames(t *testing.T) {
	d := newDir(t)
	ctx := context.Background()
	for _, bad := range []string{"", "Lea", "1lea", "lé", "admin", "a b", "this-name-is-far-too-long-for-a-user"} {
		if _, err := d.AddUser(ctx, "admin", bad, "", "", nil); !errors.Is(err, ErrBadName) {
			t.Errorf("%q: err = %v", bad, err)
		}
	}
	if _, err := d.AddUser(ctx, "admin", "lea", "", "#zzz", nil); !errors.Is(err, ErrBadName) {
		t.Errorf("a bad colour: %v", err)
	}
	if _, err := d.AddUser(ctx, "admin", "lea", "", "", []string{"nope"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown group: %v", err)
	}
	if _, err := d.AddUser(ctx, "admin", "lea", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddUser(ctx, "admin", "lea", "", "", nil); !errors.Is(err, ErrNameTaken) {
		t.Errorf("twice: %v", err)
	}
}

func TestRemoveKeepsTheRowAndTheNameTaken(t *testing.T) {
	d := newDir(t)
	ctx := context.Background()
	d.AddGroup(ctx, "admin", "famille", "", "")
	d.AddUser(ctx, "admin", "lea", "", "", []string{"famille"})
	if err := d.Remove(ctx, "admin", "lea"); err != nil {
		t.Fatal(err)
	}
	u, err := d.User(ctx, "lea")
	if err != nil || u.Active() || u.RemovedAt == nil {
		t.Fatalf("after remove: %+v, %v", u, err)
	}
	if all, _ := d.Users(ctx); len(all) != 0 {
		t.Errorf("Users lists a removed user: %v", names(all))
	}
	if ms, _ := d.Members(ctx, "famille"); len(ms) != 0 {
		t.Errorf("still a member: %v", names(ms))
	}
	if _, err := d.AddUser(ctx, "admin", "lea", "", "", nil); !errors.Is(err, ErrNameTaken) {
		t.Errorf("a removed user's name was reused: %v", err)
	}
	if err := d.Remove(ctx, "admin", "lea"); !errors.Is(err, ErrNotFound) {
		t.Errorf("removed twice: %v", err)
	}
}

func TestDisable(t *testing.T) {
	d := newDir(t)
	ctx := context.Background()
	d.AddUser(ctx, "admin", "lea", "", "", nil)
	if err := d.SetDisabled(ctx, "admin", "lea", true); err != nil {
		t.Fatal(err)
	}
	if u, _ := d.User(ctx, "lea"); u.Active() {
		t.Error("a disabled user is active")
	}
	d.SetDisabled(ctx, "admin", "lea", false)
	if u, _ := d.User(ctx, "lea"); !u.Active() {
		t.Error("an enabled user is not active")
	}
}
