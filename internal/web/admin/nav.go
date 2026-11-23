package admin

import "net/url"

// The admin UI's page tree, as claudication's and cafard's: every page has
// a path, and the path is the tree.
//
//	/                          the overview
//	/users                     the users
//	/users/<name>              one user's overview
//	/users/<name>/<page>       one of its pages (devices, calendars, ...)
//	/groups, /groups/<name>[/<page>]   the same for a group
//	/activity /backups /settings       the other sections
//
// The rail lists the choices of the page's own level: the sections at the
// top, an item's pages with the way back on an item. The trail is every
// level above the page, each a link.

type navLink struct {
	Href    string
	Label   string
	Icon    string // an SVG path, from icons.ts
	Current bool
	Name    bool // the item's crumb: its label is filled in by the page
}

type nav struct {
	Back  *navLink
	Items []navLink
	Trail []navLink
}

type navPage struct{ id, label, icon string }

const (
	icUsers    = "M9 11.2a3.2 3.2 0 100-6.4 3.2 3.2 0 000 6.4z M3 19c.6-3.3 3-5 6-5s5.4 1.7 6 5 M16 5.5a3 3 0 010 5.8 M18 14c1.7.6 2.7 2.4 3 5"
	icGroups   = "M12 10a3 3 0 100-6 3 3 0 000 6z M6 20c.5-3 2.8-5 6-5s5.5 2 6 5 M5 9.5a2.2 2.2 0 100-4.4 M19 9.5a2.2 2.2 0 110-4.4"
	icActivity = "M8 6h12 M8 12h12 M8 18h12 M4 6h.01 M4 12h.01 M4 18h.01"
	icBackups  = "M4 6c0-1.7 3.6-3 8-3s8 1.3 8 3-3.6 3-8 3-8-1.3-8-3z M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6 M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"
	icSettings = "M12 15a3 3 0 100-6 3 3 0 000 6z M12 2v3 M12 19v3 M4.2 4.2l2.1 2.1 M17.7 17.7l2.1 2.1 M2 12h3 M19 12h3 M4.2 19.8l2.1-2.1 M17.7 6.3l2.1-2.1"
	icDevices  = "M4 6a2 2 0 012-2h10a2 2 0 012 2v2 M2 18h11 M4 6v9 M15 10a1 1 0 011-1h5a1 1 0 011 1v10a1 1 0 01-1 1h-5a1 1 0 01-1-1z M18 18h1"
	icCalendar = "M4 7a2 2 0 012-2h12a2 2 0 012 2v12a2 2 0 01-2 2H6a2 2 0 01-2-2z M4 10h16 M8 3v4 M16 3v4"
	icPerson   = "M12 11.5a3.5 3.5 0 100-7 3.5 3.5 0 000 7z M5 20c.8-3.8 3.6-6 7-6s6.2 2.2 7 6"
	icBack     = "M15 5l-7 7 7 7"
	icShare    = "M18 8a3 3 0 100-6 3 3 0 000 6z M6 15a3 3 0 100-6 3 3 0 000 6z M18 22a3 3 0 100-6 3 3 0 000 6z M8.6 13.5l6.8 4 M15.4 6.5l-6.8 4"
)

var sections = []navPage{
	{"users", "Utilisateurs", icUsers},
	{"groups", "Groupes", icGroups},
	{"calendars", "Calendriers", icCalendar},
	{"activity", "Activité", icActivity},
	{"backups", "Sauvegardes", icBackups},
	{"settings", "Réglages", icSettings},
}

// itemPages are an item's pages beside its overview, in rail order.
var itemPages = map[string][]navPage{
	"users": {
		{"devices", "Appareils", icDevices},
		{"calendars", "Calendriers", icCalendar},
		{"groups", "Groupes", icGroups},
		{"activity", "Activité", icActivity},
	},
	"groups": {
		{"members", "Membres", icPerson},
		{"calendars", "Calendriers", icCalendar},
	},
	// A calendar is an item of its own, with an owner and members the admin
	// manages (the user's call).
	"calendars": {
		{"shares", "Membres", icShare},
		{"activity", "Activité", icActivity},
	},
}

// moved are item pages that became part of another: the old path lands on it.
var moved = map[string]string{"users/signin": "devices"}

// knownPage reports whether sub is one of a section's item pages ("" is the overview).
func knownPage(section, sub string) bool {
	if sub == "" {
		return true
	}
	for _, p := range itemPages[section] {
		if p.id == sub {
			return true
		}
	}
	return false
}

func labelOf(list []navPage, id string) (string, string) {
	for _, p := range list {
		if p.id == id {
			return p.label, p.icon
		}
	}
	return id, ""
}

// navOf is the rail and the trail of a page: section is the top level
// ("" for the overview), name the item, sub the item's page.
func navOf(section, name, sub string) nav {
	n := nav{Trail: []navLink{{Href: "/", Label: "Vue d'ensemble", Current: section == ""}}}
	if section != "" {
		label, icon := labelOf(sections, section)
		n.Trail = append(n.Trail, navLink{Href: "/" + section, Label: label, Icon: icon, Current: name == ""})
	}
	if name != "" {
		base := "/" + section + "/" + url.PathEscape(name)
		label, icon := labelOf(sections, section)
		n.Back = &navLink{Href: "/" + section, Label: label, Icon: icBack}
		_ = icon
		n.Trail = append(n.Trail, navLink{Href: base, Label: name, Name: true, Current: sub == ""})
		for _, p := range itemPages[section] {
			n.Items = append(n.Items, navLink{Href: base + "/" + p.id, Label: p.label, Icon: p.icon, Current: p.id == sub})
		}
		if sub != "" {
			label, _ := labelOf(itemPages[section], sub)
			n.Trail = append(n.Trail, navLink{Href: base + "/" + sub, Label: label, Current: true})
		}
		return n
	}
	for _, s := range sections {
		n.Items = append(n.Items, navLink{Href: "/" + s.id, Label: s.label, Icon: s.icon, Current: s.id == section})
	}
	return n
}
