package bootstrap

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// The data is the product.
//
// A house job that somebody did and that passed verification is a fact about
// the world with a photograph behind it: this business was open at this hour
// with this sign up, this storefront was empty. Collected, that is the first
// dataset the exchange produces, and the thing a mapping company, an insurer
// or a retail compliance team would pay for. It is written durably the
// moment it exists, because a finding that lives in memory is not a dataset.

// findingsFile is the name inside the data dir.
const findingsFile = "findings.jsonl"

// Finding is one verified answer. It carries no address beyond what the job
// published and no coordinate finer than a kilometre.
type Finding struct {
	Place    string `json:"place"`
	Job      string `json:"job"`
	Shape    Shape  `json:"shape,omitempty"`
	Question string `json:"question"`
	// Verdict is "yes" when the predicate held and "no" when it did not.
	// Evidence of "no" is paid the same as "yes", so it is kept the same.
	Verdict  string    `json:"verdict"`
	PhotoSHA string    `json:"photo_sha,omitempty"`
	At       time.Time `json:"at"`
	Area     string    `json:"area,omitempty"`
	Lat      float64   `json:"lat"`
	Lon      float64   `json:"lon"`
}

// Findings is the store.
type Findings struct {
	mu   sync.Mutex
	path string
	all  []Finding
}

// openFindings loads what is on disk. An empty dir keeps findings in memory.
func openFindings(dir string) (*Findings, error) {
	f := &Findings{}
	if dir == "" {
		return f, nil
	}
	f.path = filepath.Join(dir, findingsFile)
	fh, err := os.Open(f.path)
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		var x Finding
		if json.Unmarshal(sc.Bytes(), &x) == nil {
			f.all = append(f.all, x)
		}
	}
	return f, sc.Err()
}

// Add appends one finding, to disk first.
func (f *Findings) Add(x Finding) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.path != "" {
		line, err := json.Marshal(x)
		if err != nil {
			return err
		}
		fh, err := os.OpenFile(f.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err != nil {
			return err
		}
		defer fh.Close()
		if _, err := fh.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	f.all = append(f.all, x)
	return nil
}

// Count is how many there are.
func (f *Findings) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.all)
}

// Recent returns up to n findings, newest first.
func (f *Findings) Recent(n int) []Finding {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Finding, 0, n)
	for i := len(f.all) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, f.all[i])
	}
	return out
}

// coarse rounds a stored coordinate to two decimal places, the same grain the
// public board uses for a job's dot on the map.
func coarse(e7 int64) float64 { return math.Round(api.Deg(e7)*100) / 100 }
