The app bundle is built into this directory by `npm run build`.

app.js is not committed, but this file is. Go's embed refuses a directory
with no embeddable files, so without something committed here a clean
checkout would fail to compile before the frontend had ever been built. A
dotfile will not do: embed skips those too. This file is not served.
