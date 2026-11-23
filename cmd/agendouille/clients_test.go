package main

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/nebuloss/agendouille/internal/calendar"
)

// TestClientRequests replays internal/dav/testdata/clients against a seeded
// server: each client's requests, in order, must get the answers listed
// beside them (the README there says how they were made).
func TestClientRequests(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "dav", "testdata", "clients")
	clients, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range clients {
		if !c.IsDir() {
			continue
		}
		t.Run(c.Name(), func(t *testing.T) {
			x := startServer(t)
			x.publicOn()
			ctx := context.Background()
			lea, err := x.svc.dir.AddUser(ctx, "cli", "lea", "Léa", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			dev, _ := x.svc.auth.NewDevice(ctx, lea.ID, "other", c.Name(), "readwrite")
			if _, err := x.svc.calendar.Create(ctx, calendar.Operator("cli"), "lea", "perso", calendar.Props{}); err != nil {
				t.Fatal(err)
			}
			if _, _, err := x.svc.calendar.Put(ctx, calendar.Operator("cli"), calendar.Ref{Owner: "lea", Slug: "perso"}, "judo.ics", []byte(judo), calendar.Precondition{}); err != nil {
				t.Fatal(err)
			}
			steps, _ := filepath.Glob(filepath.Join(root, c.Name(), "*.http"))
			sort.Strings(steps)
			for _, step := range steps {
				replay(t, x, step, dev.Secret)
			}
		})
	}
}

func replay(t *testing.T, x *e2e, path, secret string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	br := bufio.NewReader(f)
	first, _ := br.ReadString('\n')
	method, target, _ := strings.Cut(strings.TrimSpace(first), " ")
	headers := http.Header{}
	for {
		line, err := br.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if line == "" || err != nil {
			break
		}
		k, v, _ := strings.Cut(line, ":")
		headers.Set(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	body, _ := io.ReadAll(br)
	a := x.do(method, x.public+target, strings.NewReader(string(body)), func(r *http.Request) {
		for k, vs := range headers {
			r.Header[k] = vs
		}
		r.SetBasicAuth("lea", secret)
	})
	want, err := os.ReadFile(strings.TrimSuffix(path, ".http") + ".want")
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(path)
	for _, line := range strings.Split(strings.TrimSpace(string(want)), "\n") {
		check, arg, _ := strings.Cut(line, " ")
		switch check {
		case "status":
			if n, _ := strconv.Atoi(arg); a.status != n {
				t.Errorf("%s: status %d, want %d\n%s", name, a.status, n, a.body)
			}
		case "contains":
			if !strings.Contains(a.body, arg) {
				t.Errorf("%s: the answer lacks %s\n%s", name, arg, a.body)
			}
		case "lacks":
			if strings.Contains(a.body, arg) {
				t.Errorf("%s: the answer holds %s\n%s", name, arg, a.body)
			}
		default:
			t.Fatalf("%s: unknown check %q", name, line)
		}
	}
}
