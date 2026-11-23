package ical

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Component is one BEGIN:X ... END:X block: its properties in the order
// they came, and the blocks inside it.
type Component struct {
	Name     string // upper case: VCALENDAR, VEVENT, VTIMEZONE, VALARM...
	Props    []Prop
	Children []*Component
}

// Prop is one content line (RFC 5545 3.1): NAME;PARAM=V,V:VALUE. Value is
// as written, still escaped; Text unescapes it.
type Prop struct {
	Name   string              // upper case
	Params map[string][]string // names upper case, values unquoted
	Value  string
}

// Param is the first value of a parameter, or "".
func (p *Prop) Param(name string) string {
	if v := p.Params[strings.ToUpper(name)]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// Text is the value with the TEXT escapes of RFC 5545 3.3.11 undone.
func (p *Prop) Text() string { return unescapeText(p.Value) }

// Get is the first property named name, or nil.
func (c *Component) Get(name string) *Prop {
	for i := range c.Props {
		if c.Props[i].Name == name {
			return &c.Props[i]
		}
	}
	return nil
}

// All lists every property named name.
func (c *Component) All(name string) []*Prop {
	var out []*Prop
	for i := range c.Props {
		if c.Props[i].Name == name {
			out = append(out, &c.Props[i])
		}
	}
	return out
}

// Text is the unescaped value of the first property named name, or "".
func (c *Component) Text(name string) string {
	if p := c.Get(name); p != nil {
		return p.Text()
	}
	return ""
}

func unescapeText(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n', 'N':
			b.WriteByte('\n')
		default: // \\ \; \, and anything a client escaped needlessly
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// EscapeText applies the TEXT escapes, for values this package writes.
func EscapeText(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`).Replace(s)
}

// maxDepth bounds nesting: VCALENDAR > VEVENT > VALARM is three.
const maxDepth = 8

// Decode reads one or more VCALENDAR blocks. Line ends may be CRLF or LF;
// folded lines (RFC 5545 3.1) are unfolded; names are matched without
// regard to case. It is strict about structure and lenient about the rest,
// as real clients need.
func Decode(data []byte) ([]*Component, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) {
		return nil, invalid("not UTF-8")
	}
	var roots []*Component
	var stack []*Component
	for n, line := range unfold(string(data)) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		p, err := parseLine(line)
		if err != nil {
			return nil, invalid("line %d: %v", n+1, err)
		}
		switch p.Name {
		case "BEGIN":
			if len(stack) == maxDepth {
				return nil, invalid("line %d: components nested too deep", n+1)
			}
			c := &Component{Name: strings.ToUpper(strings.TrimSpace(p.Value))}
			if len(stack) == 0 {
				if c.Name != "VCALENDAR" {
					return nil, invalid("line %d: %s outside a VCALENDAR", n+1, c.Name)
				}
				roots = append(roots, c)
			} else {
				top := stack[len(stack)-1]
				top.Children = append(top.Children, c)
			}
			stack = append(stack, c)
		case "END":
			name := strings.ToUpper(strings.TrimSpace(p.Value))
			if len(stack) == 0 || stack[len(stack)-1].Name != name {
				return nil, invalid("line %d: END:%s does not close the open component", n+1, name)
			}
			stack = stack[:len(stack)-1]
		default:
			if len(stack) == 0 {
				return nil, invalid("line %d: a property outside a VCALENDAR", n+1)
			}
			top := stack[len(stack)-1]
			top.Props = append(top.Props, p)
		}
	}
	if len(stack) > 0 {
		return nil, invalid("%s is never closed", stack[len(stack)-1].Name)
	}
	if len(roots) == 0 {
		return nil, invalid("no VCALENDAR")
	}
	return roots, nil
}

// unfold splits on line ends and joins continuation lines (those starting
// with a space or a tab) to the line before.
func unfold(s string) []string {
	raw := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(raw))
	var cur strings.Builder
	have := false
	for _, l := range raw {
		l = strings.TrimSuffix(l, "\r")
		if have && len(l) > 0 && (l[0] == ' ' || l[0] == '\t') {
			cur.WriteString(l[1:]) // a builder: thousands of continuations stay linear
			continue
		}
		if have {
			out = append(out, cur.String())
			cur.Reset()
		}
		cur.WriteString(l)
		have = true
	}
	if have {
		out = append(out, cur.String())
	}
	return out
}

// parseLine reads NAME *(;PARAM=VALUE[,VALUE]) : VALUE. A parameter value
// may be quoted, and then hold ; : and ,.
func parseLine(line string) (Prop, error) {
	i := strings.IndexAny(line, ";:")
	if i <= 0 {
		return Prop{}, fmt.Errorf("no property name")
	}
	p := Prop{Name: strings.ToUpper(line[:i])}
	if !validName(p.Name) {
		return Prop{}, fmt.Errorf("bad property name %q", line[:i])
	}
	for line[i] == ';' {
		i++
		eq := strings.IndexByte(line[i:], '=')
		if eq <= 0 {
			return Prop{}, fmt.Errorf("parameter without a value in %s", p.Name)
		}
		name := strings.ToUpper(line[i : i+eq])
		i += eq + 1
		var values []string
		for {
			var v string
			if i < len(line) && line[i] == '"' {
				end := strings.IndexByte(line[i+1:], '"')
				if end < 0 {
					return Prop{}, fmt.Errorf("unclosed quote in %s", p.Name)
				}
				v = line[i+1 : i+1+end]
				i += end + 2
			} else {
				end := strings.IndexAny(line[i:], ",;:")
				if end < 0 {
					return Prop{}, fmt.Errorf("no value after the parameters of %s", p.Name)
				}
				v = line[i : i+end]
				i += end
			}
			values = append(values, v)
			if i >= len(line) {
				return Prop{}, fmt.Errorf("no value after the parameters of %s", p.Name)
			}
			if line[i] != ',' {
				break
			}
			i++
		}
		if p.Params == nil {
			p.Params = map[string][]string{}
		}
		p.Params[name] = append(p.Params[name], values...)
	}
	if line[i] != ':' {
		return Prop{}, fmt.Errorf("no value in %s", p.Name)
	}
	p.Value = line[i+1:]
	return p, nil
}

func validName(s string) bool {
	for _, r := range s {
		if !(r == '-' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return s != ""
}

// Encode writes components as iCalendar text with CRLF line ends, folded
// at 75 octets on character boundaries (RFC 5545 3.1). Used for what
// agendouille assembles itself (import, export); what clients PUT is
// stored as they sent it.
func Encode(cs ...*Component) []byte {
	var b bytes.Buffer
	for _, c := range cs {
		encode(&b, c)
	}
	return b.Bytes()
}

func encode(b *bytes.Buffer, c *Component) {
	writeLine(b, "BEGIN:"+c.Name)
	for _, p := range c.Props {
		writeLine(b, p.line())
	}
	for _, child := range c.Children {
		encode(b, child)
	}
	writeLine(b, "END:"+c.Name)
}

func (p Prop) line() string {
	var s strings.Builder
	s.WriteString(p.Name)
	for _, name := range sortedKeys(p.Params) {
		s.WriteString(";" + name + "=")
		for i, v := range p.Params[name] {
			if i > 0 {
				s.WriteByte(',')
			}
			if strings.ContainsAny(v, ";:,") {
				s.WriteString(`"` + strings.ReplaceAll(v, `"`, "") + `"`)
			} else {
				s.WriteString(v)
			}
		}
	}
	s.WriteByte(':')
	s.WriteString(p.Value)
	return s.String()
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func writeLine(b *bytes.Buffer, line string) {
	const limit = 75
	first := true
	for len(line) > 0 {
		max := limit
		if !first {
			max = limit - 1 // the leading space counts
		}
		if len(line) <= max {
			if !first {
				b.WriteByte(' ')
			}
			b.WriteString(line)
			break
		}
		cut := max
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		if !first {
			b.WriteByte(' ')
		}
		b.WriteString(line[:cut])
		b.WriteString("\r\n")
		line = line[cut:]
		first = false
	}
	b.WriteString("\r\n")
}
