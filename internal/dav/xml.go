package dav

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// The namespaces of CONTRACTS.md 4, and the prefixes answers use.
const (
	nsDAV    = "DAV:"
	nsCalDAV = "urn:ietf:params:xml:ns:caldav"
	nsCS     = "http://calendarserver.org/ns/"
	nsApple  = "http://apple.com/ns/ical/"
)

var prefixes = map[string]string{nsDAV: "d", nsCalDAV: "c", nsCS: "cs", nsApple: "a"}

// maxBody caps XML request bodies (PLAN.md 10).
const maxBody = 1 << 20

// node is a parsed XML element: requests are small, so a tree is simpler
// than one struct per report.
type node struct {
	Name     xml.Name
	Attr     []xml.Attr
	Children []*node
	Text     string
	text     strings.Builder // filled while parsing: many small chunks stay linear
}

func (n *node) is(space, local string) bool {
	return n != nil && n.Name.Space == space && n.Name.Local == local
}

// child is the first child named space:local, or nil.
func (n *node) child(space, local string) *node {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.is(space, local) {
			return c
		}
	}
	return nil
}

func (n *node) attr(local string) string {
	for _, a := range n.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

var errEmpty = errors.New("empty body")

// parse reads an XML body into a tree. encoding/xml resolves no external
// entities, so nothing in a body reaches the file system or the network.
func parse(r io.Reader) (*node, error) {
	dec := xml.NewDecoder(io.LimitReader(r, maxBody))
	var stack []*node
	var root *node
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) > 32 {
				return nil, errors.New("XML nested too deep")
			}
			n := &node{Name: t.Name, Attr: t.Attr}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("two root elements")
				}
				root = n
			} else {
				p := stack[len(stack)-1]
				p.Children = append(p.Children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			top := stack[len(stack)-1]
			top.Text = top.text.String()
			top.text.Reset()
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write(t)
			}
		}
	}
	if root == nil {
		return nil, errEmpty
	}
	return root, nil
}

// propNames lists the elements inside a <d:prop>.
func propNames(prop *node) []xml.Name {
	var out []xml.Name
	if prop == nil {
		return out
	}
	for _, c := range prop.Children {
		out = append(out, c.Name)
	}
	return out
}

// writer builds a multistatus answer.
type writer struct{ b strings.Builder }

func newMultistatus() *writer {
	w := &writer{}
	w.b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	w.b.WriteString(`<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav" xmlns:cs="http://calendarserver.org/ns/" xmlns:a="http://apple.com/ns/ical/">`)
	return w
}

func (w *writer) raw(s string) { w.b.WriteString(s) }

func (w *writer) text(s string) { _ = xml.EscapeText(&w.b, []byte(s)) }

func (w *writer) finish() string {
	w.b.WriteString(`</d:multistatus>`)
	return w.b.String()
}

// open, close and empty write an element in any namespace: the four of
// CONTRACTS.md 4 by prefix, any other with its own declaration, so a
// property a client asked for comes back under its own name.
func open(name xml.Name) string {
	if p, ok := prefixes[name.Space]; ok {
		return "<" + p + ":" + name.Local + ">"
	}
	return `<x:` + name.Local + ` xmlns:x="` + escapeAttr(name.Space) + `">`
}

func closeTag(name xml.Name) string {
	if p, ok := prefixes[name.Space]; ok {
		return "</" + p + ":" + name.Local + ">"
	}
	return "</x:" + name.Local + ">"
}

func empty(name xml.Name) string {
	if p, ok := prefixes[name.Space]; ok {
		return "<" + p + ":" + name.Local + "/>"
	}
	return `<x:` + name.Local + ` xmlns:x="` + escapeAttr(name.Space) + `"/>`
}

func escapeAttr(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return strings.ReplaceAll(b.String(), `"`, "&quot;")
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func href(path string) string { return "<d:href>" + esc(path) + "</d:href>" }

// response writes one <d:response> with a propstat per status.
func (w *writer) response(path string, found map[xml.Name]string, order []xml.Name, missing []xml.Name, forbidden []xml.Name) {
	w.raw("<d:response>" + href(path))
	if len(found) > 0 {
		w.raw("<d:propstat><d:prop>")
		for _, n := range order {
			if v, ok := found[n]; ok {
				if v == "" {
					w.raw(empty(n))
				} else {
					w.raw(open(n) + v + closeTag(n))
				}
			}
		}
		w.raw("</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>")
	}
	for _, group := range []struct {
		names  []xml.Name
		status string
	}{{missing, "404 Not Found"}, {forbidden, "403 Forbidden"}} {
		if len(group.names) == 0 {
			continue
		}
		w.raw("<d:propstat><d:prop>")
		for _, n := range group.names {
			w.raw(empty(n))
		}
		w.raw("</d:prop><d:status>HTTP/1.1 " + group.status + "</d:status></d:propstat>")
	}
	w.raw("</d:response>")
}

// status writes a response with no properties, only a status (a deleted
// object in sync-collection, a missing href in multiget).
func (w *writer) status(path, status string) {
	w.raw("<d:response>" + href(path) + "<d:status>HTTP/1.1 " + status + "</d:status></d:response>")
}

// davError is a precondition failure body (RFC 4918 16).
func davError(space, local, inner string) string {
	el := xml.Name{Space: space, Local: local}
	body := empty(el)
	if inner != "" {
		body = open(el) + inner + closeTag(el)
	}
	return `<?xml version="1.0" encoding="utf-8"?>` + "\n" +
		`<d:error xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">` + body + `</d:error>`
}
