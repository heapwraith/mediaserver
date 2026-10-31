package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestBackupEndpoint(t *testing.T) {
	srv, _, _ := newLibraryServer(t)
	call := func(method, path, body string, headers ...string) (int, string) {
		return do(t, srv, method, path, body, headers...)
	}
	// The database holds every profile's progress: a profile is needed.
	if status, _ := call(http.MethodGet, "/api/admin/backup", ""); status != http.StatusUnauthorized {
		t.Fatalf("backup without profile = %d, want 401", status)
	}
	_, header := newProfile(t, call, "Sala", "")

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/admin/backup", nil)
	if err != nil {
		t.Fatal(err)
	}
	k, v, _ := strings.Cut(header, ": ")
	req.Header.Set(k, v)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := make([]byte, 16)
	if _, err := res.Body.Read(buf); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || string(buf) != "SQLite format 3\x00" {
		t.Fatalf("backup = %d %q", res.StatusCode, buf)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="mediaserver-`) {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

func TestStatusEndpoint(t *testing.T) {
	srv, sc, _ := newLibraryServer(t, "Filmes/Alien (1979)/Alien (1979).mkv", "Series/Dark/Season 1/Dark.S01E01.mkv")
	if _, err := sc.Scan(t.Context()); err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body string, headers ...string) (int, string) {
		return do(t, srv, method, path, body, headers...)
	}
	if status, _ := call(http.MethodGet, "/api/admin/status", ""); status != http.StatusUnauthorized {
		t.Fatalf("status without profile = %d, want 401", status)
	}
	_, header := newProfile(t, call, "Sala", "")
	status, body := call(http.MethodGet, "/api/admin/status", "", header)
	if status != http.StatusOK {
		t.Fatalf("status = %d %s", status, body)
	}
	s := decode[statusBody](t, body)
	if s.Version != "test" || s.Slots.Limit != 1 || s.Sessions == nil || s.Memory.Bytes == 0 {
		t.Errorf("status = %+v", s)
	}
	if s.Library.Movies != 1 || s.Library.Series != 1 || s.Library.Files != 2 {
		t.Errorf("library = %+v", s.Library)
	}
}
