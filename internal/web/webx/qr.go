package webx

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"strings"

	"rsc.io/qr"
)

// QRSVG draws text as a QR code in SVG: one path of black modules on a
// white square with a quiet zone, and no text nodes, so nothing in text
// can become markup.
func QRSVG(text string) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	const quiet = 4
	n := code.Size
	var path strings.Builder
	// One rectangle per horizontal run of black modules.
	for y := range n {
		for x := 0; x < n; {
			if !code.Black(x, y) {
				x++
				continue
			}
			run := 1
			for x+run < n && code.Black(x+run, y) {
				run++
			}
			fmt.Fprintf(&path, "M%d %dh%dv1h-%dz", x+quiet, y+quiet, run, run)
			x += run
		}
	}
	side := n + 2*quiet
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`+
		`<rect width="%d" height="%d" fill="#fff"/><path fill="#000" d="%s"/></svg>`, side, side, side, side, path.String()), nil
}

// QRDataURL is QRSVG as a data: URL for an <img src>, which the CSP allows
// and which never parses as markup in the page.
func QRDataURL(text string) (template.URL, error) {
	svg, err := QRSVG(text)
	if err != nil {
		return "", err
	}
	// The SVG is ours, built from numbers only, so this is not a way in.
	return template.URL("data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))), nil
}

// QRTerminal draws text as a QR code with half blocks, two rows of modules
// per line, for agendouille user enroll in a terminal.
func QRTerminal(text string) (string, error) {
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		return "", err
	}
	const quiet = 2
	n := code.Size
	black := func(x, y int) bool {
		x, y = x-quiet, y-quiet
		return x >= 0 && y >= 0 && x < n && y < n && code.Black(x, y)
	}
	var b strings.Builder
	for y := 0; y < n+2*quiet; y += 2 {
		for x := range n + 2*quiet {
			top, bottom := black(x, y), black(x, y+1)
			// Light on dark terminals and dark on light ones both scan:
			// the modules are drawn as spaces on the inverse colour.
			switch {
			case top && bottom:
				b.WriteString(" ")
			case top:
				b.WriteString("▄")
			case bottom:
				b.WriteString("▀")
			default:
				b.WriteString("█")
			}
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
