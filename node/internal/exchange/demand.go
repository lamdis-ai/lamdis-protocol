package exchange

// The register of work nobody could do.
//
// A live quote that comes back infeasible is the most valuable thing this
// exchange produces and it was being thrown away. Somebody's agent asked for
// gutters cleared at a real address, on a real morning, with real money behind
// it, and the answer was "nobody within range" — and then the question was
// forgotten. The coverage register records people offering to work somewhere;
// this records work asked for somewhere. Between them they say where to
// recruit, which is otherwise a guess.
//
// It is deliberately thinner than the coverage register, which at least has an
// email address in it. Nothing here identifies a buyer, an agent or a
// property: the coarse cell (api.CoarseE7, two decimal places, about a
// kilometre — the same grain the board and the coverage register publish), the
// kind, the skills asked for, and when. The predicate is never stored. The
// address is never stored. The credential is never stored, and there usually
// is not one, because /v1/quote is public.
//
// The published report is stricter than the coverage report in one way. That
// one names a place once five people have typed one, because five people who
// named a town is a town. This one names no place at all, ever — not because
// the threshold would be unsafe but because no name was ever collected. A
// quote carries coordinates and nothing else, so there is nothing to publish
// but a dot, and inventing a name for the dot by reverse-geocoding it would be
// adding precision this file never had.

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// Demand is one request that found no supply.
type Demand struct {
	// LatE7 and LonE7 are coarse: rounded on the way in, never stored finer,
	// for the same reason Coverage does it. A file that never held the precise
	// point cannot leak it.
	LatE7 int64 `json:"lat_e7"`
	LonE7 int64 `json:"lon_e7"`
	// Kind is observe or do.
	Kind string `json:"kind"`
	// Skills are the qualifications the work would have needed.
	Skills []api.Skill `json:"skills,omitempty"`
	// At is when it was asked.
	At time.Time `json:"at"`
}

// DemandMax is how many records the register will hold at all.
//
// A file-backed list that an unauthenticated endpoint appends to needs a
// ceiling somewhere, exactly as the coverage register does. Larger than that
// one because each record is a fraction of the size and carries no address to
// be responsible for.
const DemandMax = 100000

// DemandPerHour is how many infeasible quotes from one address are recorded.
//
// Higher than CoveragePerHour: an agent legitimately asks about a dozen
// addresses while planning one job, and refusing to record the twelfth would
// lose real signal. High enough to be generous, low enough that one caller in
// a loop cannot invent a market.
const DemandPerHour = 60

// Demands is the register, backed by a file beside the coverage one.
type Demands struct {
	mu   sync.Mutex
	all  []*Demand
	path string
	// seen rate-limits by caller address, the same shape Coverages uses.
	seen map[string][]time.Time
	Now  func() time.Time
}

// NewDemands loads the register, or starts empty.
func NewDemands(dir string) *Demands {
	d := &Demands{seen: map[string][]time.Time{}}
	if dir == "" {
		return d
	}
	d.path = filepath.Join(dir, "demand.json")
	if b, err := os.ReadFile(d.path); err == nil {
		json.Unmarshal(b, &d.all)
	}
	return d
}

func (ds *Demands) now() time.Time {
	if ds.Now != nil {
		return ds.Now()
	}
	return time.Now()
}

func (ds *Demands) saveLocked() {
	if ds.path == "" {
		return
	}
	b, err := json.Marshal(ds.all)
	if err != nil {
		return
	}
	tmp := ds.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		log.Printf("demand: could not write the register: %v", err)
		return
	}
	if err := os.Rename(tmp, ds.path); err != nil {
		log.Printf("demand: could not replace the register: %v", err)
	}
}

// Record files one request that could not be served, and reports whether it
// was kept.
//
// A request with no position is dropped rather than stored at 0,0: the whole
// use of this file is deciding where to recruit, and a record that cannot say
// where is a record that cannot be acted on. Everything a caller can set is
// clamped or dropped here, which is the only place it is done.
func (ds *Demands) Record(ip string, in Demand) bool {
	if !api.HasPosition(in.LatE7, in.LonE7) {
		return false
	}
	kind := in.Kind
	if kind != api.KindObserve && kind != api.KindDo {
		kind = api.KindDo
	}
	d := Demand{
		// Coarsened before it is stored, not on the way out.
		LatE7: api.CoarseE7(in.LatE7), LonE7: api.CoarseE7(in.LonE7),
		Kind:   kind,
		Skills: api.NormalizeSkills(in.Skills),
		At:     ds.now(),
	}
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if len(ds.all) >= DemandMax {
		return false
	}
	if !ds.allowLocked(ip, d.At) {
		return false
	}
	ds.all = append(ds.all, &d)
	ds.saveLocked()
	return true
}

// allowLocked reports whether this caller may file another record.
func (ds *Demands) allowLocked(ip string, now time.Time) bool {
	if ds.seen == nil {
		ds.seen = map[string][]time.Time{}
	}
	recent := ds.seen[ip][:0]
	for _, t := range ds.seen[ip] {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	if len(recent) >= DemandPerHour {
		ds.seen[ip] = recent
		return false
	}
	ds.seen[ip] = append(recent, now)
	return true
}

// Count is how many requests are on file.
func (ds *Demands) Count() int {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return len(ds.all)
}

// DemandArea is one coarse cell of the map and how much work was asked for in
// it that nobody could take.
//
// Counts are buckets, never figures, on the same ladder the coverage report
// uses and for a related reason. There the risk is naming a person; here it is
// republishing one buyer's request — "1 refrigerant job asked for at 42.33,
// -83.05 last Tuesday" is close to a job posting for a property.
type DemandArea struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
	// Asked is the bucket. AtLeast is its floor, for anything that has to
	// sort or threshold. It is a floor, not a count.
	Asked   string `json:"asked"`
	AtLeast int    `json:"at_least"`
	// Kinds is observe, do, or both. Two possible values, so it says nothing
	// about who asked even in a cell with one record in it.
	Kinds []string `json:"kinds,omitempty"`
	// Skills and LastAsked are published only once the cell is over
	// PlaceThreshold, the same threshold that gates a place name on the
	// coverage report. Below it, a trade and a date in a one-kilometre square
	// narrows to one request.
	Skills    []api.Skill `json:"skills,omitempty"`
	LastAsked string      `json:"last_asked,omitempty"`
}

// Areas reports unmet demand per coarse cell.
func (ds *Demands) Areas() []DemandArea {
	type cell struct {
		latE7, lonE7 int64
		n            int
		kinds        map[string]bool
		skills       map[api.Skill]int
		last         time.Time
	}
	cells := map[string]*cell{}
	ds.mu.Lock()
	for _, d := range ds.all {
		k := fmt.Sprintf("%d,%d", d.LatE7, d.LonE7)
		c, ok := cells[k]
		if !ok {
			c = &cell{latE7: d.LatE7, lonE7: d.LonE7,
				kinds: map[string]bool{}, skills: map[api.Skill]int{}}
			cells[k] = c
		}
		c.n++
		c.kinds[d.Kind] = true
		for _, sk := range d.Skills {
			c.skills[sk]++
		}
		if d.At.After(c.last) {
			c.last = d.At
		}
	}
	ds.mu.Unlock()

	out := make([]DemandArea, 0, len(cells))
	for _, c := range cells {
		label, floor := coverageBucket(c.n)
		a := DemandArea{
			Lat: api.Deg(c.latE7), Lon: api.Deg(c.lonE7),
			Asked: label, AtLeast: floor,
		}
		for _, k := range []string{api.KindObserve, api.KindDo} {
			if c.kinds[k] {
				a.Kinds = append(a.Kinds, k)
			}
		}
		if c.n >= PlaceThreshold {
			for sk := range c.skills {
				a.Skills = append(a.Skills, sk)
			}
			sort.Slice(a.Skills, func(i, j int) bool { return a.Skills[i] < a.Skills[j] })
			// The day, not the moment. A cell over the threshold still does
			// not need a second-accurate record of when somebody asked.
			a.LastAsked = c.last.UTC().Format("2006-01-02")
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AtLeast != out[j].AtLeast {
			return out[i].AtLeast > out[j].AtLeast
		}
		if out[i].Lat != out[j].Lat {
			return out[i].Lat < out[j].Lat
		}
		return out[i].Lon < out[j].Lon
	})
	return out
}

// DemandServer publishes the register. One route, public, read-only.
//
// Public because the people it is for are the ones who would fill the gap: an
// operator deciding whether this exchange is worth registering for, a
// contractor deciding which town to cover. Keeping it private would mean the
// signal only ever reached us, which is the arrangement that produced an empty
// board in the first place.
type DemandServer struct {
	Server *Server
}

func (dv *DemandServer) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/demand", dv.handleReport)
}

func (dv *DemandServer) handleReport(w http.ResponseWriter, r *http.Request) {
	s := dv.Server
	var areas []DemandArea
	asked := 0
	if s.Demand != nil {
		areas = s.Demand.Areas()
		asked = s.Demand.Count()
	}
	label, _ := coverageBucket(asked)
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSONResponse(w, map[string]any{
		"areas": areas,
		"asked": label,
		"note": "Work that was asked for here and that nobody within range " +
			"could take. Points are rounded to two decimal places of a degree, " +
			"about a kilometre, and counts are buckets — under five is never a " +
			"figure. No place is named because none was ever collected: a quote " +
			"carries a point and nothing else. Nothing about who asked, what " +
			"they asked for in words, or which property it concerned is stored " +
			"or published. Sandbox requests are not counted.",
	})
}
