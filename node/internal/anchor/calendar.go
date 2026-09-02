package anchor

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultCalendars are the public OpenTimestamps aggregation pools. They take
// a digest from anyone, for nothing, and commit it to Bitcoin within hours.
var DefaultCalendars = []string{
	"https://a.pool.opentimestamps.org",
	"https://b.pool.opentimestamps.org",
}

// ErrNotFound is what a calendar says about a commitment it has not yet put
// in a block: come back later.
var ErrNotFound = errors.New("ots: calendar has no timestamp for that commitment yet")

const otsAccept = "application/vnd.opentimestamps.v1"

// calendar is the minimal client: submit a digest, fetch a commitment.
type calendar struct {
	http *http.Client
}

func newCalendar(c *http.Client) *calendar {
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	return &calendar{http: c}
}

// submit posts a 32-byte digest and returns the calendar's timestamp of it.
func (c *calendar) submit(ctx context.Context, base string, digest []byte) (*Timestamp, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(base, "/")+"/digest", bytes.NewReader(digest))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", otsAccept)
	req.Header.Set("User-Agent", "lamdis-exchange-anchor")
	body, err := c.do(req)
	if err != nil {
		return nil, err
	}
	return ParseTimestamp(body, digest)
}

// get fetches the timestamp of a commitment the calendar was given earlier.
func (c *calendar) get(ctx context.Context, base string, commitment []byte) (*Timestamp, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimSuffix(base, "/")+"/timestamp/"+hex.EncodeToString(commitment), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", otsAccept)
	req.Header.Set("User-Agent", "lamdis-exchange-anchor")
	body, err := c.do(req)
	if err != nil {
		return nil, err
	}
	return ParseTimestamp(body, commitment)
}

func (c *calendar) do(req *http.Request) ([]byte, error) {
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return nil, fmt.Errorf("ots: refusing to call %q", req.URL.String())
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ots: %s answered %d: %s", req.URL.Host, resp.StatusCode,
			strings.TrimSpace(string(body)))
	}
	return body, nil
}

// upgrade follows every pending attestation to the calendar it names and
// merges whatever has since been committed. It reports whether anything
// changed. Calendars that are unreachable or not ready leave their pending
// attestation in place for next time.
func (c *calendar) upgrade(ctx context.Context, t *Timestamp) bool {
	changed := false
	for _, node := range t.pendings() {
		if node.Msg == nil {
			continue
		}
		for _, a := range node.Attestations {
			uri, ok := a.Pending()
			if !ok {
				continue
			}
			got, err := c.get(ctx, uri, node.Msg)
			if err != nil {
				continue
			}
			if _, done := got.Complete(); !done {
				// The calendar answered with more pending; nothing to keep.
				continue
			}
			if node.Merge(got) == nil {
				changed = true
			}
		}
		if _, done := node.Complete(); done {
			node.dropPending()
		}
	}
	return changed
}
