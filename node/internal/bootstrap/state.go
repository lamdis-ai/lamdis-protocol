package bootstrap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// What has to survive a restart.
//
// Two things, and both are promises. The money the house has ever drawn
// against its budget, because "never more than the budget" means across
// restarts or it means nothing. And which places have been posted, because
// "never the same place twice in a month" is the same kind of promise.

// stateFile is the name inside the data dir.
const stateFile = "bootstrap.json"

// SeenFor is how long a place is off limits after a job is posted for it.
const SeenFor = 30 * 24 * time.Hour

type state struct {
	// ToppedUpMinor is every cent ever moved from outside into the house
	// balance. It only goes up, and never past the budget.
	ToppedUpMinor int64 `json:"topped_up_minor"`
	Posted        int   `json:"posted"`
	// Seen maps a place to when a job was last posted for it.
	Seen map[string]time.Time `json:"seen"`
	// Jobs tracks the house's listings: what each asked about and where.
	Jobs     map[string]*jobRec `json:"jobs"`
	LastRun  time.Time          `json:"last_run,omitempty"`
	Clusters int                `json:"clusters"`
}

type jobRec struct {
	Place    string    `json:"place"`
	Shape    Shape     `json:"shape"`
	Question string    `json:"question"`
	Area     string    `json:"area,omitempty"`
	Cluster  string    `json:"cluster"`
	Posted   time.Time `json:"posted"`
	RadiusM  int64     `json:"radius_m"`
	Widened  bool      `json:"widened,omitempty"`
	Filled   bool      `json:"filled,omitempty"`
	Settled  bool      `json:"settled,omitempty"`
	LatE7    int64     `json:"lat_e7"`
	LonE7    int64     `json:"lon_e7"`
	Expires  time.Time `json:"expires"`
}

func newState() *state {
	return &state{Seen: map[string]time.Time{}, Jobs: map[string]*jobRec{}}
}

func loadState(dir string) (*state, error) {
	st := newState()
	if dir == "" {
		return st, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, stateFile))
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, st); err != nil {
		return nil, err
	}
	if st.Seen == nil {
		st.Seen = map[string]time.Time{}
	}
	if st.Jobs == nil {
		st.Jobs = map[string]*jobRec{}
	}
	return st, nil
}

// save writes atomically: the budget promise is only as good as the file.
func (st *state) save(dir string, now time.Time) error {
	if dir == "" {
		return nil
	}
	for id, at := range st.Seen {
		if now.Sub(at) > SeenFor {
			delete(st.Seen, id)
		}
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, stateFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, stateFile))
}

// seen reports whether a place was posted within the window.
func (st *state) seen(place string, now time.Time) bool {
	at, ok := st.Seen[place]
	return ok && now.Sub(at) <= SeenFor
}
