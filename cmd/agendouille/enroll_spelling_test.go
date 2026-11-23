package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The links and commands from before "enroll" took its two l still work
// (DECISIONS: enroll).
func TestOldEnrolSpelling(t *testing.T) {
	x := startServer(t)
	x.publicOn()
	a := x.do("GET", x.public+"/enrol/abc_DEF-123", nil, nil)
	if a.status != http.StatusMovedPermanently || a.header.Get("Location") != "/enroll/abc_DEF-123" {
		t.Errorf("/enrol/: %d %s", a.status, a.header.Get("Location"))
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "none.yaml"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{"AGENDOUILLE_CONFIG": filepath.Join(dir, "none.yaml"), "AGENDOUILLE_DATA_DIR": dir,
		"AGENDOUILLE_PUBLIC_URL": "https://agenda.example.org"}
	if _, errOut, err := runArgs(t, vars, "user", "add", "lea"); err != nil {
		t.Fatalf("user add: %v %s", err, errOut)
	}
	out, errOut, err := runArgs(t, vars, "user", "enrol", "lea")
	if err != nil || !strings.Contains(out, "/enroll/") {
		t.Errorf("user enrol: %v %q %q", err, out, errOut)
	}
}
