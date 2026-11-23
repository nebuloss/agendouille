The Android app embedded in the server (PLAN.md 7.6), put here before
`go build` by scripts/devdeploy.sh or the release workflow:

  agendouille.apk   android/build/agendouille.apk
  cert.txt          `make -C android cert`: the signing certificate's SHA-256
  version.txt       the version the APK was built as

They are not committed. This file is, because go:embed needs the
directory to hold something.
