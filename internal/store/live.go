package store

import (
	"sync"

	"modernc.org/sqlite"
)

// Every write marks what it changed (the admin UI's live updates,
// internal/live; claudication's rule "every change says what it changed").
// Rather than each write remembering to, SQLite says it: a pre-update hook
// notes the tables a transaction writes, and its commit hands them to the
// listener; a rollback drops them. So no write in any package can forget,
// and TestEveryTableHasATopic holds the table -> topic map in internal/live
// to the schema.

// listeners are told the tables each committed transaction wrote, by the
// database's DSN: two stores in one process (the tests) hear only their own.
var listeners sync.Map // dsn -> *func(tables []string)

// OnChange sets what is told of this store's committed writes (serve: the
// hub); nil tells nobody.
func (s *Store) OnChange(fn func(tables []string)) {
	if fn == nil {
		listeners.Delete(s.dsn)
		return
	}
	listeners.Store(s.dsn, &fn)
}

var hookOnce sync.Once

// installHooks registers, once per process, the hooks on every connection
// the driver opens.
func installHooks() {
	hookOnce.Do(func() {
		sqlite.RegisterConnectionHook(func(conn sqlite.ExecQuerierContext, dsn string) error {
			h, ok := conn.(sqlite.HookRegisterer)
			if !ok {
				return nil
			}
			var mu sync.Mutex
			pending := map[string]bool{}
			h.RegisterPreUpdateHook(func(d sqlite.SQLitePreUpdateData) {
				mu.Lock()
				pending[d.TableName] = true
				mu.Unlock()
			})
			h.RegisterCommitHook(func() int32 {
				mu.Lock()
				tables := make([]string, 0, len(pending))
				for t := range pending {
					tables = append(tables, t)
				}
				clear(pending)
				mu.Unlock()
				if fn, ok := listeners.Load(dsn); ok && len(tables) > 0 {
					(*fn.(*func([]string)))(tables)
				}
				return 0 // 0: the commit goes ahead
			})
			h.RegisterRollbackHook(func() {
				mu.Lock()
				clear(pending)
				mu.Unlock()
			})
			return nil
		})
	})
}
