package store

import (
	"context"
	"database/sql"
	"sort"
	"sync"
	"testing"

	"github.com/nebuloss/agendouille/internal/live"
)

// Every table the migrations make says which topics a write to it changes
// (internal/live, Tables): a table added without its topics is a screen
// that never moves.
func TestEveryTableHasATopic(t *testing.T) {
	s, err := OpenDir(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows, err := s.DB().Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		if _, ok := live.Tables[name]; !ok {
			t.Errorf("table %q has no topics in internal/live/tables.go", name)
		}
	}
}

// A committed write tells the store's listener which tables it wrote; a
// rolled-back one tells nobody.
func TestCommitsAreTold(t *testing.T) {
	s, err := OpenDir(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var mu sync.Mutex
	var told [][]string
	s.OnChange(func(tables []string) {
		sort.Strings(tables)
		mu.Lock()
		told = append(told, tables)
		mu.Unlock()
	})
	ctx := context.Background()
	if _, err := s.DB().ExecContext(ctx, `INSERT INTO notice_reads(id, read_at) VALUES ('x', 'z')`); err != nil {
		t.Fatal(err)
	}
	tx, _ := s.DB().BeginTx(ctx, nil)
	_, _ = tx.ExecContext(ctx, `INSERT INTO notice_reads(id, read_at) VALUES ('gone', 'z')`)
	_ = tx.Rollback()
	err = InTx(ctx, s.DB(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE notice_reads SET read_at = 'w' WHERE id = 'x'`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, detail) VALUES ('t', 'a', 'k', 'x', '')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(told) != 2 || told[0][0] != "notice_reads" || len(told[1]) != 2 || told[1][0] != "audit" || told[1][1] != "notice_reads" {
		t.Errorf("told: %v", told)
	}
	if topics := live.ForTables([]string{"changes", "objects", "audit"}); len(topics) != 2 {
		t.Errorf("topics of changes, objects and audit: %v", topics)
	}
}
