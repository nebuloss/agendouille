package i18n

import "fmt"

// catalog is every message the server writes, by key, in each language. It
// is assembled from the catalog_*.go files, one per area, so each keeps its
// own keys: layout., app., admin., notice., setting., error.
var catalog = map[string]map[Lang]string{}

// register adds an area's messages; a key defined twice is a programming
// error, caught at start and by the tests.
func register(m map[string]map[Lang]string) {
	for k, v := range m {
		if _, dup := catalog[k]; dup {
			panic(fmt.Sprintf("i18n: %q is defined twice", k))
		}
		catalog[k] = v
	}
}

func init() {
	register(map[string]map[Lang]string{
		"layout.noscript": {
			FR: "agendouille a besoin de JavaScript pour fonctionner.",
			EN: "agendouille needs JavaScript to work.",
		},
		"layout.source": {
			FR: "Code source sur GitHub",
			EN: "Source on GitHub",
		},
		"layout.loading": {
			FR: "Chargement…",
			EN: "Loading…",
		},
	})
}
