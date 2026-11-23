package assets

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func get(t *testing.T, mux *http.ServeMux, path string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code
}

func TestABundleIsNeverServedOnTheOtherSurface(t *testing.T) {
	admin := http.NewServeMux()
	if err := Register(admin, Admin); err != nil {
		t.Fatal(err)
	}
	if code := get(t, admin, App.URL()); code != http.StatusNotFound {
		t.Errorf("the app bundle on the admin mux: %d, want 404", code)
	}
	app := http.NewServeMux()
	if err := Register(app, App); err != nil {
		t.Fatal(err)
	}
	if code := get(t, app, Admin.URL()); code != http.StatusNotFound {
		t.Errorf("the admin bundle on the app mux: %d, want 404", code)
	}
}

func TestEachBundleIsServedOnItsSurfaceWhenBuilt(t *testing.T) {
	for _, b := range []Bundle{App, Admin} {
		mux := http.NewServeMux()
		if err := Register(mux, b); err != nil {
			t.Fatal(err)
		}
		want := http.StatusNotFound
		if Built(b) {
			want = http.StatusOK
		}
		if code := get(t, mux, b.URL()); code != want {
			t.Errorf("%s: %d, want %d (built: %v)", b, code, want, Built(b))
		}
	}
}

func TestTheEmbedPlaceholdersAreNotServed(t *testing.T) {
	mux := http.NewServeMux()
	if err := Register(mux, App); err != nil {
		t.Fatal(err)
	}
	if code := get(t, mux, "/static/app/README.txt"); code != http.StatusNotFound {
		t.Errorf("README.txt: %d, want 404", code)
	}
}
