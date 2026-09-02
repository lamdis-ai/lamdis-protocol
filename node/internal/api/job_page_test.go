package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A dispatch email, a pushed offer and a shared link all name one job. Both
// the JSON and the page for it have to open for whoever follows the link,
// signed in or not, and neither may open for directed work — which is not on
// the board and has no business being confirmed to a stranger.
func TestReadJobIsPublicForOpenWorkOnly(t *testing.T) {
	b, _, srv, _ := newBoardServer(t)
	postTask(t, b, "open-1", 1)
	if err := b.Post(&Listing{
		Job: "directed-1", Kind: KindTask, Title: "only for our vendor",
		Where: "1 Private Way", PayMinor: 500, Currency: "USD", Slots: 1,
		DirectedTo: []string{"vendor-x"},
		Expires:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	code, body := do(t, srv, "GET", "/v1/board/open-1", nil, nil)
	if code != http.StatusOK {
		t.Fatalf("unauthenticated read of an open job returned %d: %s", code, body)
	}
	var got Listing
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Job != "open-1" || got.Title == "" {
		t.Errorf("read returned %+v, want the listing", got)
	}
	if got.Where != "" || got.Instructions != "" {
		t.Error("the public read leaks the address or instructions")
	}

	if code, _ := do(t, srv, "GET", "/v1/board/directed-1", nil, nil); code != http.StatusNotFound {
		t.Errorf("directed work read returned %d, want 404", code)
	}
	if code, _ := do(t, srv, "GET", "/v1/board/nope", nil, nil); code != http.StatusNotFound {
		t.Errorf("unknown job read returned %d, want 404", code)
	}
}

func TestJobPageRendersOpenWork(t *testing.T) {
	b, _, srv, _ := newBoardServer(t)
	postTask(t, b, "open-2", 1)
	if err := b.Post(&Listing{
		Job: "directed-2", Kind: KindTask, Title: "only for our vendor",
		Where: "1 Private Way", PayMinor: 500, Currency: "USD", Slots: 1,
		DirectedTo: []string{"vendor-x"},
		Expires:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	res, err := http.Get(srv.URL + "/j/open-2")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("job page returned %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("job page is %q, want HTML", ct)
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)
	if !strings.Contains(page, "a FOR LEASE sign is up at 742 Evergreen") {
		t.Error("the page does not carry the job's title")
	}
	if strings.Contains(page, "742 Evergreen Rd") {
		t.Error("the page carries the exact address, which only the claimant may see")
	}
	if !strings.Contains(page, `"/v1/board/"`) {
		t.Error("the page does not read the job from /v1/board/{job}")
	}

	for _, id := range []string{"nope", "directed-2"} {
		if code, _ := do(t, srv, "GET", "/j/"+id, nil, nil); code != http.StatusNotFound {
			t.Errorf("/j/%s returned %d, want 404", id, code)
		}
	}
}

// The queue links each listing to its page, so a row can be shared.
func TestBoardRowsLinkToJobPage(t *testing.T) {
	if !strings.Contains(boardPageHTML, `href="/j/' + encodeURIComponent(w.job)`) {
		t.Error("board rows do not link to /j/{job}")
	}
}
