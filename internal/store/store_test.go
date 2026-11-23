package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenMigratesOnceAndTightensModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	ctx := context.Background()
	s, err := OpenDir(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.DB().QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil || n == 0 {
		t.Fatalf("migrations recorded: %d, %v", n, err)
	}
	s.Close()

	// Opening again applies nothing new and keeps the data.
	s, err = OpenDir(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var again int
	if err := s.DB().QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&again); err != nil || again != n {
		t.Fatalf("second open: %d migrations, want %d (%v)", again, n, err)
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, s.Path(): 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s: mode %v, want %v", path, info.Mode().Perm(), want)
		}
	}
}

func TestForeignKeysAreOn(t *testing.T) {
	s, err := OpenDir(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.DB().Exec(`INSERT INTO group_members(group_id, user_id) VALUES (42, 42)`)
	if err == nil {
		t.Fatal("a member of no group was inserted: foreign keys are off")
	}
}

func TestTimeRoundTripsAndSortsAsText(t *testing.T) {
	a := time.Date(2026, 11, 23, 9, 0, 0, 5, time.FixedZone("CET", 3600))
	b := a.Add(time.Nanosecond)
	got, err := ParseTime(Time(a))
	if err != nil || !got.Equal(a) {
		t.Fatalf("round trip: %v, %v", got, err)
	}
	if !(Time(a) < Time(b)) {
		t.Errorf("%s should sort before %s", Time(a), Time(b))
	}
}
