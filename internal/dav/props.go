package dav

import (
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nebuloss/agendouille/internal/calendar"
	"github.com/nebuloss/agendouille/internal/ical"
	"github.com/nebuloss/agendouille/internal/sharing"
)

func dn(local string) xml.Name { return xml.Name{Space: nsDAV, Local: local} }
func cn(local string) xml.Name { return xml.Name{Space: nsCalDAV, Local: local} }

var (
	pResourceType     = dn("resourcetype")
	pDisplayName      = dn("displayname")
	pCurrentPrincipal = dn("current-user-principal")
	pPrincipalURL     = dn("principal-URL")
	pOwner            = dn("owner")
	pPrivileges       = dn("current-user-privilege-set")
	pReports          = dn("supported-report-set")
	pSyncToken        = dn("sync-token")
	pETag             = dn("getetag")
	pContentType      = dn("getcontenttype")
	pContentLength    = dn("getcontentlength")
	pLastModified     = dn("getlastmodified")
	pHomeSet          = cn("calendar-home-set")
	pUserAddressSet   = cn("calendar-user-address-set")
	pComponentSet     = cn("supported-calendar-component-set")
	pCalendarData     = cn("calendar-data")
	pSupportedData    = cn("supported-calendar-data")
	pMaxSize          = cn("max-resource-size")
	pDescription      = cn("calendar-description")
	pCalendarTimezone = cn("calendar-timezone")
	pCTag             = xml.Name{Space: nsCS, Local: "getctag"}
	pColor            = xml.Name{Space: nsApple, Local: "calendar-color"}
	pOrder            = xml.Name{Space: nsApple, Local: "calendar-order"}
	pPrincipalColSet  = dn("principal-collection-set")
)

// resource is one thing a multistatus reports on.
type resource struct {
	kind kind
	path string
	cal  *calendar.Info
	obj  *calendar.Object
}

// props are the properties a resource has, as inner XML ("" = empty
// element), in the order allprop lists them. withData adds calendar-data.
func (c *call) props(res resource, withData bool) (map[xml.Name]string, []xml.Name) {
	user := c.p.User.Name
	out := map[xml.Name]string{}
	var order []xml.Name
	put := func(n xml.Name, v string) { out[n] = v; order = append(order, n) }
	put(pCurrentPrincipal, href(principalPath(user)))
	switch res.kind {
	case kRoot:
		put(pResourceType, "<d:collection/>")
		put(pPrincipalColSet, href("/dav/principals/"))
	case kPrincipal:
		put(pResourceType, "<d:collection/><d:principal/>")
		put(pDisplayName, esc(c.p.User.DisplayName))
		put(pPrincipalURL, href(principalPath(user)))
		put(pHomeSet, href(homePath(user)))
		put(pUserAddressSet, href(principalPath(user)))
		put(pPrincipalColSet, href("/dav/principals/"))
	case kHome:
		put(pResourceType, "<d:collection/>")
		put(pDisplayName, esc(c.p.User.DisplayName))
		put(pOwner, href(principalPath(user)))
		priv := "<d:privilege><d:read/></d:privilege>"
		if c.p.DeviceCap != "read" {
			priv += "<d:privilege><d:bind/></d:privilege>"
		}
		put(pPrivileges, priv)
	case kCalendar:
		i := res.cal
		put(pResourceType, "<d:collection/><c:calendar/>")
		put(pDisplayName, esc(i.DisplayName()))
		put(pOwner, href(principalPath(i.Ref.Owner)))
		put(pDescription, esc(i.Description))
		put(pColor, esc(i.DisplayColor()))
		order := 0
		if i.View.Order != nil {
			order = *i.View.Order
		}
		put(pOrder, strconv.Itoa(order))
		priv := "<d:privilege><d:read/></d:privilege>"
		if i.Access.CanWrite() {
			priv += "<d:privilege><d:write/></d:privilege><d:privilege><d:write-content/></d:privilege>" +
				"<d:privilege><d:write-properties/></d:privilege><d:privilege><d:bind/></d:privilege><d:privilege><d:unbind/></d:privilege>"
		}
		put(pPrivileges, priv)
		put(pReports, "<d:supported-report><d:report><c:calendar-multiget/></d:report></d:supported-report>"+
			"<d:supported-report><d:report><c:calendar-query/></d:report></d:supported-report>"+
			"<d:supported-report><d:report><d:sync-collection/></d:report></d:supported-report>")
		put(pComponentSet, `<c:comp name="VEVENT"/>`)
		put(pSupportedData, `<c:calendar-data content-type="text/calendar" version="2.0"/>`)
		put(pMaxSize, strconv.Itoa(c.s.d.Settings.Get().MaxObjectBytes))
		put(pCTag, strconv.FormatInt(i.Seq, 10))
		put(pSyncToken, esc(calendar.Token(c.s.d.PublicURL(), *i)))
		put(pContentType, "text/calendar; charset=utf-8; component=vcalendar")
	case kObject:
		o := res.obj
		put(pResourceType, "")
		put(pETag, esc(o.ETag))
		put(pContentType, "text/calendar; charset=utf-8; component=vevent")
		put(pContentLength, strconv.Itoa(len(o.Data)))
		put(pLastModified, o.ModifiedAt.UTC().Format(http.TimeFormat))
		if withData {
			put(pCalendarData, esc(string(o.Data)))
		}
	}
	return out, order
}

// propRequest is what a PROPFIND or REPORT asks for.
type propRequest struct {
	all   bool
	names bool // propname: names only
	props []xml.Name
}

func readPropRequest(root *node) propRequest {
	switch {
	case root == nil || root.child(nsDAV, "allprop") != nil:
		return propRequest{all: true}
	case root.child(nsDAV, "propname") != nil:
		return propRequest{names: true}
	}
	return propRequest{props: propNames(root.child(nsDAV, "prop"))}
}

func (q propRequest) wantsData() bool {
	for _, n := range q.props {
		if n == pCalendarData {
			return true
		}
	}
	return false
}

// write answers one resource for a property request.
func (c *call) write(w *writer, res resource, q propRequest) {
	have, order := c.props(res, q.wantsData())
	switch {
	case q.all:
		w.response(res.path, have, order, nil, nil)
	case q.names:
		names := map[xml.Name]string{}
		for _, n := range order {
			names[n] = ""
		}
		w.response(res.path, names, order, nil, nil)
	default:
		found := map[xml.Name]string{}
		var missing []xml.Name
		for _, n := range q.props {
			if v, ok := have[n]; ok {
				found[n] = v
			} else {
				missing = append(missing, n)
			}
		}
		w.response(res.path, found, q.props, missing, nil)
	}
}

func (c *call) propfind() {
	depth := c.r.Header.Get("Depth")
	if strings.EqualFold(depth, "infinity") {
		c.xmlError(http.StatusForbidden, nsDAV, "propfind-finite-depth", "")
		return
	}
	root, err := parse(c.r.Body)
	if err != nil && !errors.Is(err, errEmpty) {
		http.Error(c.w, "the body is not XML", http.StatusBadRequest)
		return
	}
	if root != nil && !root.is(nsDAV, "propfind") {
		http.Error(c.w, "PROPFIND takes a propfind body", http.StatusBadRequest)
		return
	}
	q := readPropRequest(root)
	ctx := c.r.Context()
	user := c.p.User.Name
	w := newMultistatus()
	switch c.t.kind {
	case kRoot:
		c.write(w, resource{kind: kRoot, path: "/dav/"}, q)
	case kPrincipal:
		c.write(w, resource{kind: kPrincipal, path: principalPath(user)}, q)
	case kHome:
		c.write(w, resource{kind: kHome, path: homePath(user)}, q)
		if depth == "1" {
			cals, err := c.s.d.Calendar.Home(ctx, c.a, false)
			if err != nil {
				c.fail(err)
				return
			}
			for i := range cals {
				c.write(w, resource{kind: kCalendar, path: calendarPath(user, cals[i].Ref), cal: &cals[i]}, q)
			}
		}
	case kCalendar:
		i, err := c.info()
		if err != nil {
			c.fail(err)
			return
		}
		c.write(w, resource{kind: kCalendar, path: c.calHref(i), cal: &i}, q)
		if depth == "1" {
			objs, err := c.s.d.Calendar.List(ctx, i)
			if err != nil {
				c.fail(err)
				return
			}
			for k := range objs {
				c.write(w, resource{kind: kObject, path: c.objHref(i, objs[k].Name), obj: &objs[k]}, q)
			}
		}
	case kObject:
		ref, err := c.t.ref()
		if err != nil {
			c.fail(err)
			return
		}
		i, o, err := c.s.d.Calendar.Get(ctx, c.a, ref, c.t.name)
		if err != nil {
			c.fail(err)
			return
		}
		c.write(w, resource{kind: kObject, path: c.objHref(i, o.Name), obj: &o}, q)
	}
	c.multistatus(w.finish())
}

func (c *call) report() {
	root, err := parse(c.r.Body)
	if err != nil {
		http.Error(c.w, "the body is not XML", http.StatusBadRequest)
		return
	}
	if c.t.kind != kCalendar {
		c.xmlError(http.StatusForbidden, nsDAV, "supported-report", "")
		return
	}
	i, err := c.info()
	if err != nil {
		c.fail(err)
		return
	}
	switch {
	case root.is(nsCalDAV, "calendar-multiget"):
		c.multiget(i, root)
	case root.is(nsCalDAV, "calendar-query"):
		c.query(i, root)
	case root.is(nsDAV, "sync-collection"):
		c.syncCollection(i, root)
	default:
		c.xmlError(http.StatusForbidden, nsDAV, "supported-report", "")
	}
}

// calHref is a calendar's href as this request names it: under the bare
// slug when the client used its MKCALENDAR URL (PLAN.md 5.2's fallback; a
// client that insists on its own URL matches answers by it), else under
// <owner>~<slug>.
func (c *call) calHref(i calendar.Info) string {
	if (c.t.kind == kCalendar || c.t.kind == kObject) && !strings.Contains(c.t.calSeg, "~") && i.Ref.Owner == c.p.User.Name {
		return homePath(c.p.User.Name) + url.PathEscape(c.t.calSeg) + "/"
	}
	return calendarPath(c.p.User.Name, i.Ref)
}

func (c *call) objHref(i calendar.Info, name string) string {
	return c.calHref(i) + url.PathEscape(name)
}

// nameOf reads the object an href names inside calendar i, from any of the
// paths it is listed under in the user's home; "" when it names none.
func (c *call) nameOf(i calendar.Info, h string) string {
	u, err := url.Parse(strings.TrimSpace(h))
	if err != nil {
		return ""
	}
	t := route(u.Path)
	if t.kind != kObject || t.user != c.p.User.Name {
		return ""
	}
	if r, err := t.ref(); err != nil || r != i.Ref {
		return ""
	}
	return t.name
}

func (c *call) multiget(i calendar.Info, root *node) {
	q := readPropRequest(root)
	var names []string
	var hrefs []string
	for _, ch := range root.Children {
		if ch.is(nsDAV, "href") {
			hrefs = append(hrefs, strings.TrimSpace(ch.Text))
			names = append(names, c.nameOf(i, ch.Text))
		}
	}
	found, err := c.s.d.Calendar.GetMany(c.r.Context(), i, names)
	if err != nil {
		c.fail(err)
		return
	}
	w := newMultistatus()
	for k, n := range names {
		o, ok := found[n]
		if !ok {
			w.status(hrefs[k], "404 Not Found")
			continue
		}
		c.write(w, resource{kind: kObject, path: c.objHref(i, o.Name), obj: &o}, q)
	}
	c.multistatus(w.finish())
}

// filter is the part of calendar-query agendouille supports (PLAN.md 5.2).
type filter struct {
	none       bool // a component the calendar never holds (VTODO): no match
	from, to   *time.Time
	propFilter []propFilter
}

type propFilter struct {
	name        string
	isNotDef    bool
	text        string
	negate      bool
	caseInsens  bool
	hasTextTest bool
}

var errFilter = errors.New("unsupported filter")

func parseFilter(f *node) (filter, error) {
	var out filter
	if f == nil {
		return out, nil
	}
	cal := f.child(nsCalDAV, "comp-filter")
	if cal == nil {
		return out, nil
	}
	if !strings.EqualFold(cal.attr("name"), "VCALENDAR") {
		return out, errFilter
	}
	for _, comp := range cal.Children {
		if !comp.is(nsCalDAV, "comp-filter") {
			continue
		}
		if !strings.EqualFold(comp.attr("name"), "VEVENT") {
			out.none = true // VTODO, VJOURNAL: never in this calendar
			return out, nil
		}
		if comp.child(nsCalDAV, "is-not-defined") != nil {
			out.none = true
			return out, nil
		}
		for _, ch := range comp.Children {
			switch {
			case ch.is(nsCalDAV, "time-range"):
				parseT := func(s string) (*time.Time, error) {
					if s == "" {
						return nil, nil
					}
					t, err := time.Parse("20060102T150405Z", s)
					return &t, err
				}
				var err error
				if out.from, err = parseT(ch.attr("start")); err != nil {
					return out, errFilter
				}
				if out.to, err = parseT(ch.attr("end")); err != nil {
					return out, errFilter
				}
			case ch.is(nsCalDAV, "prop-filter"):
				pf := propFilter{name: strings.ToUpper(ch.attr("name"))}
				if ch.child(nsCalDAV, "is-not-defined") != nil {
					pf.isNotDef = true
				} else if tm := ch.child(nsCalDAV, "text-match"); tm != nil {
					pf.hasTextTest = true
					pf.text = tm.Text
					pf.negate = strings.EqualFold(tm.attr("negate-condition"), "yes")
					pf.caseInsens = tm.attr("collation") != "i;octet"
				}
				if ch.child(nsCalDAV, "time-range") != nil || ch.child(nsCalDAV, "param-filter") != nil {
					return out, errFilter
				}
				out.propFilter = append(out.propFilter, pf)
			case ch.is(nsCalDAV, "comp-filter"):
				// VALARM filters: no client of PLAN.md 5.4 sends them.
				return out, errFilter
			}
		}
	}
	return out, nil
}

// matches applies the prop-filters to an object's events.
func (f filter) matches(o calendar.Object) bool {
	if len(f.propFilter) == 0 {
		return true
	}
	roots, err := ical.Decode(o.Data)
	if err != nil || len(roots) == 0 {
		return false
	}
	for _, ev := range roots[0].Children {
		if ev.Name != "VEVENT" {
			continue
		}
		ok := true
		for _, pf := range f.propFilter {
			p := ev.Get(pf.name)
			switch {
			case pf.isNotDef:
				ok = ok && p == nil
			case !pf.hasTextTest:
				ok = ok && p != nil
			default:
				if p == nil {
					ok = false
					continue
				}
				v, want := p.Text(), pf.text
				if pf.caseInsens {
					v, want = strings.ToLower(v), strings.ToLower(want)
				}
				hit := strings.Contains(v, want)
				ok = ok && hit != pf.negate
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func (c *call) query(i calendar.Info, root *node) {
	q := readPropRequest(root)
	f, err := parseFilter(root.child(nsCalDAV, "filter"))
	if err != nil {
		c.xmlError(http.StatusForbidden, nsCalDAV, "supported-filter", "")
		return
	}
	w := newMultistatus()
	if f.none {
		c.multistatus(w.finish())
		return
	}
	ctx := c.r.Context()
	var objs []calendar.Object
	if f.from != nil || f.to != nil {
		from, to := time.Unix(0, 0).UTC(), time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
		if f.from != nil {
			from = *f.from
		}
		if f.to != nil {
			to = *f.to
		}
		objs, err = c.s.d.Calendar.Range(ctx, i, from, to)
	} else {
		objs, err = c.s.d.Calendar.List(ctx, i)
	}
	if err != nil {
		c.fail(err)
		return
	}
	for k := range objs {
		if !f.matches(objs[k]) {
			continue
		}
		c.write(w, resource{kind: kObject, path: c.objHref(i, objs[k].Name), obj: &objs[k]}, q)
	}
	c.multistatus(w.finish())
}

func (c *call) syncCollection(i calendar.Info, root *node) {
	q := readPropRequest(root)
	token := ""
	if t := root.child(nsDAV, "sync-token"); t != nil {
		token = strings.TrimSpace(t.Text)
	}
	limit := 0
	if l := root.child(nsDAV, "limit"); l != nil {
		if n := l.child(nsDAV, "nresults"); n != nil {
			limit, _ = strconv.Atoi(strings.TrimSpace(n.Text))
		}
	}
	base := c.s.d.PublicURL()
	var since int64
	if token != "" {
		var err error
		if since, err = calendar.ParseToken(base, i, token); err != nil {
			c.fail(err)
			return
		}
	}
	ch, err := c.s.d.Calendar.ChangesSince(c.r.Context(), i, since, limit)
	if err != nil {
		c.fail(err)
		return
	}
	found, err := c.s.d.Calendar.GetMany(c.r.Context(), i, ch.Names)
	if err != nil {
		c.fail(err)
		return
	}
	w := newMultistatus()
	for _, n := range ch.Names {
		path := c.objHref(i, n)
		if o, ok := found[n]; ok {
			c.write(w, resource{kind: kObject, path: path, obj: &o}, q)
		} else {
			w.status(path, "404 Not Found")
		}
	}
	if ch.Truncated {
		w.raw("<d:response>" + href(c.calHref(i)) +
			"<d:status>HTTP/1.1 507 Insufficient Storage</d:status><d:error><d:number-of-matches-within-limits/></d:error></d:response>")
	}
	next := i
	next.Seq = ch.Seq
	w.raw("<d:sync-token>" + esc(calendar.Token(base, next)) + "</d:sync-token>")
	c.multistatus(w.finish())
}

// calendarProps reads the properties of a MKCALENDAR or MKCOL body.
func calendarProps(prop *node) (calendar.Props, []string, bool) {
	var p calendar.Props
	var comps []string
	isCalendar := true
	if prop == nil {
		return p, nil, true
	}
	for _, ch := range prop.Children {
		v := ch.Text
		switch ch.Name {
		case pDisplayName:
			p.Name = &v
		case pColor:
			p.Color = &v
		case pDescription:
			p.Description = &v
		case pCalendarTimezone:
			if tz := tzidOf(v); tz != "" {
				p.Timezone = &tz
			}
		case pComponentSet:
			for _, comp := range ch.Children {
				comps = append(comps, strings.ToUpper(comp.attr("name")))
			}
		case pResourceType:
			isCalendar = ch.child(nsCalDAV, "calendar") != nil
		}
	}
	return p, comps, isCalendar
}

// tzidOf is the TZID of the VTIMEZONE a client gives as calendar-timezone,
// if Go knows it.
func tzidOf(v string) string {
	roots, err := ical.Decode([]byte(v))
	if err != nil {
		return ""
	}
	for _, c := range roots[0].Children {
		if c.Name == "VTIMEZONE" {
			tz := c.Text("TZID")
			if _, err := time.LoadLocation(tz); err == nil {
				return tz
			}
		}
	}
	return ""
}

// mkcalendar creates a calendar in the user's own home (PLAN.md 5.2). The
// client asked for /dav/calendars/<user>/<slug>/; the calendar lives at
// <user>~<slug>, said in Location, and the bare slug keeps answering.
func (c *call) mkcalendar() {
	if c.t.kind != kCalendar {
		c.w.Header().Set("Allow", allow)
		http.Error(c.w, "a calendar is made inside your home", http.StatusMethodNotAllowed)
		return
	}
	if c.p.DeviceCap == "read" {
		c.xmlError(http.StatusForbidden, nsDAV, "need-privileges", "")
		return
	}
	ref, err := c.t.ref()
	if err != nil || ref.Owner != c.p.User.Name {
		c.xmlError(http.StatusForbidden, nsDAV, "need-privileges", "")
		return
	}
	root, err := parse(c.r.Body)
	if err != nil && !errors.Is(err, errEmpty) {
		http.Error(c.w, "the body is not XML", http.StatusBadRequest)
		return
	}
	var prop *node
	if root != nil {
		if set := root.child(nsDAV, "set"); set != nil {
			prop = set.child(nsDAV, "prop")
		}
	}
	p, comps, isCalendar := calendarProps(prop)
	if c.r.Method == "MKCOL" && (root == nil || !isCalendar) {
		c.xmlError(http.StatusForbidden, nsDAV, "valid-resourcetype", "")
		return
	}
	if len(comps) > 0 {
		hasEvent := false
		for _, comp := range comps {
			hasEvent = hasEvent || comp == "VEVENT"
		}
		if !hasEvent {
			c.xmlError(http.StatusForbidden, nsCalDAV, "supported-calendar-component", "")
			return
		}
	}
	i, err := c.s.d.Calendar.Create(c.r.Context(), c.a, c.p.User.Name, ref.Slug, p)
	if err != nil {
		c.fail(err)
		return
	}
	c.w.Header().Set("Location", calendarPath(c.p.User.Name, i.Ref))
	c.w.Header().Set("Cache-Control", "no-cache")
	c.w.WriteHeader(http.StatusCreated)
}

// proppatch changes a calendar's properties: the owner's for everyone, a
// sharee's for their own devices (PLAN.md 5.2). It is all or nothing (RFC
// 4918 9.2): a property that cannot be set as asked is 403 and every other
// one 424, and nothing changes. A property answered 200 was really set.
func (c *call) proppatch() {
	root, err := parse(c.r.Body)
	if err != nil || !root.is(nsDAV, "propertyupdate") {
		http.Error(c.w, "PROPPATCH takes a propertyupdate body", http.StatusBadRequest)
		return
	}
	if c.p.DeviceCap == "read" {
		c.xmlError(http.StatusForbidden, nsDAV, "need-privileges", "")
		return
	}
	path := c.r.URL.Path
	type op struct {
		name   xml.Name
		value  string
		remove bool
	}
	var ops []op
	for _, o := range root.Children {
		remove := o.is(nsDAV, "remove")
		if !remove && !o.is(nsDAV, "set") {
			continue
		}
		for _, ch := range o.child(nsDAV, "prop").childrenOrNil() {
			ops = append(ops, op{name: ch.Name, value: ch.Text, remove: remove})
		}
	}
	names := make([]xml.Name, 0, len(ops))
	for _, o := range ops {
		names = append(names, o.name)
	}
	w := newMultistatus()
	if c.t.kind != kCalendar {
		w.response(path, nil, nil, nil, names)
		c.multistatus(w.finish())
		return
	}
	i, err := c.info()
	if err != nil {
		c.fail(err)
		return
	}
	owner := i.Access == sharing.Owner
	var props calendar.Props
	var refused []xml.Name
	for _, o := range ops {
		v := o.value
		ok := true
		switch o.name {
		case pDisplayName:
			// A sharee may drop their own name for it; the owner's is the
			// calendar's, which cannot be removed.
			if o.remove && owner {
				ok = false
			} else if o.remove {
				v = ""
			}
			props.Name = &v
		case pColor:
			if o.remove && owner {
				ok = false
			} else if o.remove {
				v = ""
			} else if _, valid := calendar.NormalizeColor(v); !valid {
				ok = false
			}
			props.Color = &v
		case pDescription:
			if !owner {
				ok = false // the description is the owner's, there is no view of it
			}
			if o.remove {
				v = ""
			}
			props.Description = &v
		case pOrder:
			n := 0
			if !o.remove {
				var err error
				if n, err = strconv.Atoi(strings.TrimSpace(v)); err != nil {
					ok = false
				}
			}
			props.Order = &n
		case pCalendarTimezone:
			tz := tzidOf(v)
			if !owner || o.remove || tz == "" {
				ok = false
			}
			props.Timezone = &tz
		default:
			ok = false
		}
		if !ok {
			refused = append(refused, o.name)
		}
	}
	if len(refused) > 0 {
		var failed []xml.Name
		for _, n := range names {
			if !containsName(refused, n) {
				failed = append(failed, n)
			}
		}
		w.raw("<d:response>" + href(path))
		w.raw("<d:propstat><d:prop>")
		for _, n := range refused {
			w.raw(empty(n))
		}
		w.raw("</d:prop><d:status>HTTP/1.1 403 Forbidden</d:status></d:propstat>")
		if len(failed) > 0 {
			w.raw("<d:propstat><d:prop>")
			for _, n := range failed {
				w.raw(empty(n))
			}
			w.raw("</d:prop><d:status>HTTP/1.1 424 Failed Dependency</d:status></d:propstat>")
		}
		w.raw("</d:response>")
		c.multistatus(w.finish())
		return
	}
	if _, err := c.s.d.Calendar.Update(c.r.Context(), c.a, i.Ref, props); err != nil {
		c.fail(err)
		return
	}
	ok := map[xml.Name]string{}
	for _, n := range names {
		ok[n] = ""
	}
	w.response(path, ok, names, nil, nil)
	c.multistatus(w.finish())
}

func (n *node) childrenOrNil() []*node {
	if n == nil {
		return nil
	}
	return n.Children
}

func containsName(list []xml.Name, n xml.Name) bool {
	for _, x := range list {
		if x == n {
			return true
		}
	}
	return false
}
