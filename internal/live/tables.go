package live

// Tables says which topics a write to each table changes: what the store's
// hooks mark when a transaction commits (internal/store, live.go). Every
// table of the schema is here; TestEveryTableHasATopic fails on one that is
// not. An empty list says on purpose that no screen shows it.
var Tables = map[string][]Topic{
	"users":             {People},
	"groups":            {People},
	"group_members":     {People},
	"devices":           {Devices},
	"enrollments":       {Devices},
	"approvals":         {Devices},
	"sessions":          {Devices},
	"links":             {Devices},
	"link_calendars":    {Devices},
	"calendars":         {Calendars},
	"calendar_views":    {Calendars},
	"shares":            {Calendars},
	"objects":           {Calendars},
	"changes":           {Calendars, Activity},
	"audit":             {Activity},
	"switches":          {Settings},
	"settings":          {Settings},
	"notice_reads":      {Notices},
	"admin":             {}, // the admin password: no screen shows it
	"schema_migrations": {},
}

// ForTables are the topics writes to tables change, each once.
func ForTables(tables []string) []Topic {
	seen := map[Topic]bool{}
	var out []Topic
	for _, t := range tables {
		for _, topic := range Tables[t] {
			if !seen[topic] {
				seen[topic] = true
				out = append(out, topic)
			}
		}
	}
	return out
}
