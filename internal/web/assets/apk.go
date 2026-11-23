package assets

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"strings"
)

// The Android app of this server's version (PLAN.md 7.6): built on
// dev-build or in CI by android/Makefile and copied here before go build,
// with the signing certificate's SHA-256 (for assetlinks.json) and the
// version it was built as. Absent in a build without the app: the server
// then offers no download and no App Links.
//
//go:embed apk
var apkFiles embed.FS

// APK is the embedded Android app.
type APK struct {
	Body    []byte
	SHA256  string // of the file, hex
	Cert    string // the signing certificate's SHA-256, AA:BB:… as Android writes it
	Version string
}

var apk = loadAPK()

func loadAPK() *APK {
	body, err := apkFiles.ReadFile("apk/agendouille.apk")
	if err != nil || len(body) == 0 {
		return nil
	}
	sum := sha256.Sum256(body)
	a := &APK{Body: body, SHA256: hex.EncodeToString(sum[:])}
	if b, err := apkFiles.ReadFile("apk/cert.txt"); err == nil {
		a.Cert = strings.ToUpper(strings.TrimSpace(string(b)))
	}
	if b, err := apkFiles.ReadFile("apk/version.txt"); err == nil {
		a.Version = strings.TrimSpace(string(b))
	}
	return a
}

// AndroidApp is the embedded Android app, or nil when this build has none.
func AndroidApp() *APK { return apk }
