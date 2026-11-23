// Package i18n holds the server's own words, in French and English: the
// page templates and the messages of API errors a person reads
// (DECISIONS: languages). The web bundles have their own catalogs
// (src/shared/i18n); this one is for what the server writes.
//
// The language is the reader's choice (the agd_lang cookie, set by the
// pages), else the first of the browser's languages we have, else French.
package i18n

import (
	"context"
	"net/http"
	"strings"
)

// Lang is a language we translate to.
type Lang string

// The languages.
const (
	FR Lang = "fr"
	EN Lang = "en"
)

// Default is French: the family's.
const Default = FR

// Cookie holds the reader's choice.
const Cookie = "agd_lang"

// Langs are the languages offered.
var Langs = []Lang{FR, EN}

func known(s string) (Lang, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) >= 2 {
		s = s[:2]
	}
	for _, l := range Langs {
		if string(l) == s {
			return l, true
		}
	}
	return "", false
}

// FromRequest is the request's language: the cookie, then
// Accept-Language in order (its q-values are in order in practice), then
// the default.
func FromRequest(r *http.Request) Lang {
	if c, err := r.Cookie(Cookie); err == nil {
		if l, ok := known(c.Value); ok {
			return l
		}
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(part, ";")
		if l, ok := known(tag); ok {
			return l
		}
	}
	return Default
}

type ctxKey struct{}

// With puts the request's language in its context, for code that only has
// the context (an error rendered deep in a handler).
func With(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxKey{}, FromRequest(r)))
}

// Of is the language in a context, or the default.
func Of(ctx context.Context) Lang {
	if l, ok := ctx.Value(ctxKey{}).(Lang); ok {
		return l
	}
	return Default
}

// T is the message key in lang, its {name} placeholders filled from args
// given as name, value pairs. A key missing from the catalog is returned as
// is (the tests make sure none is).
func T(l Lang, key string, args ...string) string {
	m, ok := catalog[key]
	if !ok {
		return key
	}
	text := m[l]
	if text == "" {
		text = m[Default]
	}
	for i := 0; i+1 < len(args); i += 2 {
		text = strings.ReplaceAll(text, "{"+args[i]+"}", args[i+1])
	}
	return text
}

// Has reports whether key is in the catalog.
func Has(key string) bool {
	_, ok := catalog[key]
	return ok
}
