package ical

import (
	"strings"
	"time"
)

// windowsZones maps the Windows names Outlook and Exchange put in TZID to
// IANA zones (from CLDR's windowsZones.xml, the territory "001" entries
// most likely in a family's calendar). Only the names Go cannot load
// itself are needed.
var windowsZones = map[string]string{
	"Romance Standard Time":           "Europe/Paris",
	"W. Europe Standard Time":         "Europe/Berlin",
	"Central Europe Standard Time":    "Europe/Budapest",
	"Central European Standard Time":  "Europe/Warsaw",
	"GMT Standard Time":               "Europe/London",
	"Greenwich Standard Time":         "Atlantic/Reykjavik",
	"E. Europe Standard Time":         "Europe/Chisinau",
	"FLE Standard Time":               "Europe/Kiev",
	"GTB Standard Time":               "Europe/Bucharest",
	"Russian Standard Time":           "Europe/Moscow",
	"Morocco Standard Time":           "Africa/Casablanca",
	"W. Central Africa Standard Time": "Africa/Lagos",
	"South Africa Standard Time":      "Africa/Johannesburg",
	"Eastern Standard Time":           "America/New_York",
	"Central Standard Time":           "America/Chicago",
	"Mountain Standard Time":          "America/Denver",
	"Pacific Standard Time":           "America/Los_Angeles",
	"Atlantic Standard Time":          "America/Halifax",
	"Canada Central Standard Time":    "America/Regina",
	"SA Pacific Standard Time":        "America/Bogota",
	"E. South America Standard Time":  "America/Sao_Paulo",
	"Argentina Standard Time":         "America/Buenos_Aires",
	"Hawaiian Standard Time":          "Pacific/Honolulu",
	"Tokyo Standard Time":             "Asia/Tokyo",
	"China Standard Time":             "Asia/Shanghai",
	"India Standard Time":             "Asia/Kolkata",
	"Arabian Standard Time":           "Asia/Dubai",
	"SE Asia Standard Time":           "Asia/Bangkok",
	"Singapore Standard Time":         "Asia/Singapore",
	"AUS Eastern Standard Time":       "Australia/Sydney",
	"New Zealand Standard Time":       "Pacific/Auckland",
	"UTC":                             "UTC",
	"Coordinated Universal Time":      "UTC",
}

// Zone resolves a TZID (PLAN.md 4.4): Go's own tz database first, then the
// Windows names, then some clients' "/mozilla.org/.../Europe/Paris" style
// prefixes, and last the calendar's zone. The object's VTIMEZONE stays in
// the text either way; only the instants computed here depend on this.
func Zone(tzid string, fallback *time.Location) *time.Location {
	tzid = strings.Trim(strings.TrimSpace(tzid), `"`)
	if loc, err := time.LoadLocation(tzid); err == nil && tzid != "" && tzid != "Local" {
		return loc
	}
	if name, ok := windowsZones[tzid]; ok {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	// "/mozilla.org/20050126_1/Europe/Paris", "/citadel.org/.../Europe/Paris"
	if parts := strings.Split(tzid, "/"); len(parts) >= 2 {
		for i := 1; i < len(parts); i++ {
			candidate := strings.Join(parts[i:], "/")
			if loc, err := time.LoadLocation(candidate); err == nil && strings.Contains(candidate, "/") {
				return loc
			}
		}
	}
	return fallback
}
