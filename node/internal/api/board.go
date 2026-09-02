package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// The board is the marketplace: the place where work that nobody has been
// assigned yet can be found by whoever wants it.
//
// Until now every path into this system was a push — the exchange already knew
// your phone number and sent you a link. That works for a pilot and not for a
// market. A market needs a pull: somebody arrives who was never invited, sees
// what is on offer, takes one, does it, and gets paid.
//
// Two kinds of work sit on the same board because they are the same shape from
// the worker's side. A task means going and observing something and uploading
// what you saw. A review means looking at what somebody else uploaded and
// saying whether it shows what it claims. Both pay per admissible submission
// rather than per favourable answer, which is what keeps a market of strangers
// from converging on telling the buyer what they want to hear.

// Kinds of listing.
//
// The distinction between the first two is not cosmetic, and it changes how
// they must be paid.
//
// An OBSERVE job asks what is already true. The worker does not affect the
// answer, so paying only when the answer is yes would pay them to say yes —
// which is why an observation pays a fee for admissible evidence whichever way
// it turns out, plus a bonus that depends on the finding.
//
// A DO job asks the worker to make something true: put the sign up, collect
// the parcel, deliver it here. Now the worker does control the answer, and
// "paid the same either way" would mean paid for not bothering. So a DO job
// pays on completion. It still pays something for a genuine failed attempt —
// the shop was shut, the address does not exist — because a worker who travels
// and finds an impossible job has still spent their afternoon, and refusing
// them anything teaches everyone to only accept the easy ones.
const (
	KindObserve = "observe"
	KindDo      = "do"
	KindReview  = "review"

	// KindTask is the old name for an observation, kept so existing callers
	// and stored listings keep working.
	KindTask = KindObserve
)

// IsWork reports whether a kind is something a person goes and does, as
// opposed to judging somebody else's evidence.
func IsWork(kind string) bool { return kind == KindObserve || kind == KindDo }

// ReportField is one thing a worker must find out and write down.
//
// Typed rather than free text so an agent receives an answer it can act on
// without guessing: a price is a number of minor units, a date is a date, and
// "they said next Tuesday sometime" is a note beside it rather than instead
// of it. A job like "get three quotes for a water heater" is not answered by a
// photograph — it is answered by a small table.
type ReportField struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Kind     string `json:"kind"` // text, money, date, phone, url, bool
	Required bool   `json:"required,omitempty"`
	// Repeats marks a field that appears once per row, for jobs that collect
	// several of something — three quotes, five providers.
	Repeats bool `json:"repeats,omitempty"`
}

// Field kinds.
const (
	FieldText  = "text"
	FieldMoney = "money"
	FieldDate  = "date"
	FieldPhone = "phone"
	FieldURL   = "url"
	FieldBool  = "bool"
)

// Listing is one piece of open work, as a stranger sees it.
//
// It deliberately carries no secrets. The challenge code a provider must
// include in their photograph is issued at claim time, not published: a code
// visible on a public board could be composited into an old photograph by
// someone who never went anywhere.
type Listing struct {
	Job string `json:"job"`
	// Parent is the job this work judges, for reviews. It is what lets the
	// board refuse to let somebody judge their own submission.
	Parent string `json:"parent,omitempty"`
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	// Detail is the scope of the work, and it is public: nobody can price a
	// job whose size they cannot see, and an open job with no scope turns an
	// auction into guesswork.
	//
	// It must not carry access information. That is Instructions.
	Detail string `json:"detail,omitempty"`
	// Where is the exact place: a street address, a unit number, whatever
	// somebody needs to actually arrive. Never published. Released only to
	// the person who has claimed the job.
	Where string `json:"where,omitempty"`

	// Stages cut a long job into pieces that can each be evidenced and paid
	// for. Empty means a single-visit job, which is how everything behaved
	// before staged work existed.
	Stages []Stage `json:"stages,omitempty"`

	// WorkHours is how long the buyer expects the work to take, and therefore
	// how long somebody may hold it. Zero takes the board's default, which
	// suits an errand and ruins anything longer.
	WorkHours int `json:"work_hours,omitempty"`

	// NotBefore and NotAfter bound when the work may be done.
	//
	// An expiry says when a job stops being worth doing; it does not say that
	// somebody has to be home between two and four on Tuesday. Without a
	// window, every job that needs a person present is unbookable, which is
	// most of them.
	//
	// Zero means unconstrained: go whenever, before it expires.
	NotBefore time.Time `json:"not_before,omitempty"`
	NotAfter  time.Time `json:"not_after,omitempty"`

	// Area is the coarse locality shown on the open board — a neighbourhood, a
	// town, "north side of the industrial park". Enough to decide whether a
	// job is worth taking, not enough to identify the property.
	//
	// The two are separate fields because they answer different questions to
	// different people, and collapsing them is what put front doors on a
	// public endpoint.
	Area string `json:"area,omitempty"`
	// PayMinor is what an admissible submission earns regardless of the answer.
	PayMinor int64 `json:"pay_minor,omitempty"`
	// BonusMinor is the additional amount that depends on the outcome.
	BonusMinor int64  `json:"bonus_minor,omitempty"`
	Currency   string `json:"currency"`
	// Slots and Taken bound how many people can work on this.
	Slots int `json:"slots"`
	Taken int `json:"taken"`
	// Instructions is how to actually do a DO job, in the buyer's own words,
	// including how to get in: gate codes, where the key is, which door.
	// Released to the claimant and to nobody else, ever.
	//
	// Scope belongs in Detail. Putting it here hides it from bidders; putting
	// access in Detail publishes it to the internet.
	Instructions string `json:"instructions,omitempty"`
	// Deliverable describes what proof is expected — "a photo of the sign in
	// place", "the parcel at the door with the house number visible".
	Deliverable string `json:"deliverable,omitempty"`
	// AttemptMinor is what a DO job pays for a documented failed attempt. Zero
	// means nothing is paid for trying, which is a choice the buyer makes
	// visibly rather than a default.
	AttemptMinor int64 `json:"attempt_minor,omitempty"`
	// ExpenseCapMinor is how much the worker may lay out and be reimbursed for
	// against a receipt. A worker should never be asked to front money with no
	// stated ceiling.
	ExpenseCapMinor int64 `json:"expense_cap_minor,omitempty"`

	// LatE7, LonE7 and RadiusM geofence the evidence: a photograph whose own
	// metadata puts it two towns away is not evidence of this errand. Stored
	// as integer degrees times 1e7, because no money-adjacent struct in this
	// codebase carries a float.
	LatE7   int64 `json:"lat_e7,omitempty"`
	LonE7   int64 `json:"lon_e7,omitempty"`
	RadiusM int64 `json:"radius_m,omitempty"`

	// AreaLat and AreaLon are the public, coarse point: the position above
	// rounded to two decimal places, about a kilometre. Set only by Public(),
	// never by a caller, and precise enough to put a dot on the board's map
	// without being precise enough to find a front door. See Public.
	AreaLat float64 `json:"area_lat,omitempty"`
	AreaLon float64 `json:"area_lon,omitempty"`

	// Pricing is "fixed" or "bids". Empty means fixed.
	Pricing string `json:"pricing,omitempty"`
	// BidsCloseAt is when an open job stops taking offers.
	BidsCloseAt time.Time `json:"bids_close_at,omitempty"`
	// MaxBidMinor is the most the buyer will accept. It is what an open job
	// escrows against, since the real price is not known when it is posted.
	//
	// It never leaves the server on a public response. A ceiling a bidder can
	// see is a ceiling every bid lands exactly on, which turns an auction into
	// a posted price and costs the buyer the entire difference. See Public.
	MaxBidMinor int64 `json:"-"`
	// Awarded is the worker whose bid won.
	Awarded string `json:"awarded,omitempty"`

	// Report is the structured answer this job wants back, if it wants one.
	// "Get three quotes for a water heater" is not answered by a photograph;
	// it is answered by a small table, and asking for it as free text would
	// give an agent something it has to parse and might parse wrong.
	Report []ReportField `json:"report,omitempty"`

	// DistanceMiles is how far this job is from whoever is asking. Filled in
	// per request rather than stored, because it is a fact about the reader.
	DistanceMiles float64 `json:"distance_miles,omitempty"`

	// Skills are what someone must be qualified for to take this job.
	Skills []Skill `json:"skills,omitempty"`

	// DirectedTo restricts this job to named suppliers.
	//
	// Work a buyer has already assigned to their own vendor is not a market
	// event. It never reaches the open board, runs no auction, and cannot be
	// claimed by anybody else — publishing it would waste every other
	// operator's attention and tell the world who this buyer works with.
	DirectedTo []string `json:"directed_to,omitempty"`

	// Requires is what a buyer insists on before somebody may take this:
	// verified cover, a vetted supplier. Checked at claim, not advertised.
	Requires *Requirements `json:"requires,omitempty"`

	// SiteID is the buyer's own name for the location, carried through to the
	// receipt so four hundred stores can be told apart afterwards.
	SiteID string `json:"site_id,omitempty"`

	// Reference is the buyer's own identifier — a purchase order, a cost
	// centre, a work order number. The exchange never interprets it and
	// always carries it: a receipt that cannot be matched to a purchase order
	// cannot be paid by a company with an accounts department.
	Reference string `json:"reference,omitempty"`

	// ProjectID is the budget envelope this job draws on, if any.
	//
	// Published. It was carried on the listing and stripped by Public, so an
	// operator was never told that the job in front of them was one piece of a
	// larger scope at one address. See scope.go for why that is a pricing bug
	// rather than a cosmetic one.
	ProjectID string `json:"project_id,omitempty"`
	// ProjectTitle is the whole scope in the buyer's words, carried on each
	// piece so the board can say what the pieces add up to without a second
	// lookup into a store operators cannot read.
	ProjectTitle string `json:"project_title,omitempty"`
	// Project is the shape of that scope, filled in by the board on the way
	// out. Never set by a caller.
	Project *ProjectBrief `json:"project,omitempty"`
	// Brief is open text from the buyer's agent, carried verbatim to whoever
	// does the work and never interpreted by the exchange.
	//
	// An agent posting a job knows things about it that no schema here will
	// ever have a field for. Rather than guess at those fields, carry the text.
	// This is the difference between an execution layer and a project manager:
	// the exchange proves what happened, and does not pretend to understand the
	// trade.
	Brief string `json:"brief,omitempty"`

	// Access is how to get in: the gate code, which flowerpot the key is
	// under, the alarm sequence.
	//
	// Split out of Instructions, which used to carry both this and the
	// description of the work. One field holding a secret and a necessity
	// means one of them is always handled wrong, and the one that lost was
	// bidding: Instructions had to stay private, so nobody could read what the
	// job was. Released to the claimant with Where, and to nobody else.
	Access string `json:"access,omitempty"`

	// References are what the buyer supplies so the work can be priced and the
	// place found. See reference.go.
	References []Reference `json:"references,omitempty"`

	// SiteMark is what proves the photographs are of this property and not a
	// similar one somewhere else. See sitemark.go. Inferred from the address
	// when the buyer does not state one.
	SiteMark *SiteMark `json:"site_mark,omitempty"`

	// Unknowns are what the buyer cannot specify. See brief.go.
	Unknowns []Unknown `json:"unknowns,omitempty"`
	// Agreed is what the winning bid said it priced on, carried onto the job
	// so the work is judged against the figures both sides accepted.
	Agreed []Assumption `json:"agreed,omitempty"`
	// Withheld explains a redaction on the public board, so a job with no
	// visible instructions does not read as an empty one.
	Withheld string `json:"withheld,omitempty"`

	// DependsOn names jobs that must be finished before this one may start.
	//
	// Physical work has an order that is real: a slab cures before anything
	// drives on it, and you do not surface a drive the concrete truck still
	// needs to cross. Without this the order lives only in somebody's head and
	// two operators book the same ground for the same morning.
	DependsOn []string `json:"depends_on,omitempty"`
	// BidsAsOne means the buyer will consider a single offer covering every
	// piece of the project, priced per piece and awarded together.
	BidsAsOne bool `json:"bids_as_one,omitempty"`
	// BlockedBy names dependencies not yet accepted, filled in on the way out
	// so an operator sees why a piece cannot be taken today.
	BlockedBy []string `json:"blocked_by,omitempty"`

	// PlanBy says who decides how this job breaks into stages: the buyer who
	// posted it, or the supplier who wins it. Empty means the buyer, so every
	// job posted before this existed behaves exactly as it did.
	PlanBy string `json:"plan_by,omitempty"`
	// ProposedStages is a supplier's breakdown, waiting on the buyer.
	ProposedStages []Stage `json:"proposed_stages,omitempty"`
	// PlanState is "", "proposed" or "accepted".
	PlanState string `json:"plan_state,omitempty"`
	// PlanNote is why a plan was sent back.
	PlanNote string `json:"plan_note,omitempty"`
	// Cancelled marks work the buyer withdrew before anybody did it.
	Cancelled bool `json:"cancelled,omitempty"`

	// Accepted marks work that was submitted and passed verification.
	//
	// Deliberately not the same question as Finished below, which asks whether
	// a listing can still be worked. A job whose only seat was taken by
	// somebody who photographed the wrong thing is Finished and not Accepted,
	// and a dependency must read the second: "the slab has cured" cannot become
	// true because a stranger turned up and gave up.
	Accepted bool `json:"accepted,omitempty"`

	// Practice marks a job that exists so somebody can learn the flow, not
	// because anybody wants the work done.
	//
	// Seeded demonstration listings used to be indistinguishable from real
	// ones. Somebody could claim one, travel to an address, and find no bins —
	// which teaches them the board is fake, and is a worse first experience
	// than an empty board would have been. A practice run says what it is,
	// pays nothing, and can be done from a kitchen table.
	Practice bool `json:"practice,omitempty"`

	// PostedByAgent records that software, not a person, wrote this job.
	//
	// Workers on the first marketplace of this shape had no way to tell
	// whether their employer was a human or an automated pipeline, which is
	// how people ended up staffing closed-loop reconnaissance without knowing
	// there was nobody on the other end. It costs nothing to say.
	PostedByAgent bool `json:"posted_by_agent"`

	// Owner is the account whose money is escrowed against this job.
	//
	// Every buyer-side operation keys on it: reading sealed bids, awarding
	// one, reading the evidence. Without it an authenticated stranger could
	// award somebody else's job to a colluding bidder and spend their escrow,
	// which is the whole marketplace defeated by one missing comparison.
	//
	// Never published — Public strips it.
	Owner string `json:"-"`
	// Funding records where a listing's escrow came from when it was not a
	// balance: a card authorised for the job's ceiling and captured only for
	// what was actually paid out. Nil for jobs funded from a balance. Never
	// published.
	Funding *Funding `json:"-"`

	// Tier is the verification standard the submission must reach.
	Tier    string    `json:"tier,omitempty"`
	Expires time.Time `json:"expires"`
	Posted  time.Time `json:"posted"`
	// Challenge is issued to a claimant and never appears in the public list.
	Challenge string `json:"-"`
}

// Finished reports whether a listing can no longer be worked, so whatever is
// left of its escrow belongs back with the buyer.
func (l *Listing) Finished(now time.Time) bool {
	return l.Taken >= l.Slots || !now.Before(l.Expires)
}

// All returns every listing, open or not, for sweeping.
func (b *Board) All() []*Listing {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]*Listing, 0, len(b.listings))
	for _, l := range b.listings {
		cp := *l
		out = append(out, &cp)
	}
	return out
}

// Public is what a stranger may see.
//
// An open job shows no money at all. Bidders learn what the work is, where it
// is, and when offers close — and decide what it is worth to them, which is the
// only reason to run an auction rather than post a price.
//
// Redaction is done by building a new value rather than by clearing fields on
// this one, so a future field is invisible until somebody adds it here on
// purpose. The opposite arrangement leaks by default.
func (l *Listing) Public() *Listing {
	p := &Listing{
		Job: l.Job, Parent: l.Parent, Kind: l.Kind,
		Title: l.Title,
		// Where and Access are deliberately absent.
		//
		// A buyer's agent writes what somebody needs to get in — a gate code,
		// which flowerpot the key is under, an alarm sequence. Publishing that
		// on an unauthenticated endpoint, alongside the street address, hands
		// it to anybody who curls the board. The sealed-bid design took great
		// care to hide the price and was publishing the front door.
		//
		// That used to mean withholding Instructions, because entry details
		// lived in them — which left a bidder unable to read the work they
		// were pricing. Access is now its own field, so the secret can stay
		// secret without the job being illegible.
		//
		// The claimant gets both, over the capability, once they hold the job.
		Area: l.Area,
		// What the work is, and what would prove it. Both are published, and
		// both have to be: an operator asked to name a price without them is
		// guessing, and an auction of guesses is worse than a fixed price.
		//
		// Access is the part that stays private, and it is now its own field
		// rather than a paragraph buried in these.
		// Instructions, Brief and Detail are filled in below, after screening.
		Deliverable: l.Deliverable,
		Unknowns:    l.Unknowns,
		// Published deliberately. A bidder who cannot see the site is
		// guessing, and an auction of guesses is worse than a fixed price.
		References: l.References,
		Currency:   l.Currency, Slots: l.Slots, Taken: l.Taken,
		Tier: l.Tier, Expires: l.Expires, Posted: l.Posted,
		Pricing: l.Pricing, BidsCloseAt: l.BidsCloseAt,
		DistanceMiles: l.DistanceMiles, Skills: l.Skills,
		NotBefore: l.NotBefore, NotAfter: l.NotAfter,
		Stages: l.Stages, WorkHours: l.WorkHours,
		// Project membership is published; the budget behind it is not.
		ProjectID: l.ProjectID, ProjectTitle: l.ProjectTitle,
		DependsOn: l.DependsOn, BidsAsOne: l.BidsAsOne,
		// Whether the winner writes the schedule is a term of the job, and an
		// operator decides whether to bid partly on it.
		PlanBy: l.PlanBy, PlanState: l.PlanState,
		PostedByAgent: l.PostedByAgent, Practice: l.Practice,
		SiteID: l.SiteID,
		Report: l.Report,
		// Exact coordinates are deliberately absent.
		//
		// Removing the street address from the board is undone by publishing
		// the same property to seven decimal places, which is roughly a
		// centimetre. Distance is computed server-side and returned as
		// DistanceMiles. What the board does need is a dot on a map, and a
		// dot rounded to two decimal places (about a kilometre, the same
		// grain as Area) says which part of town without saying which house.
		// That is AreaLat and AreaLon, set below.
		ExpenseCapMinor: l.ExpenseCapMinor,
	}
	if HasPosition(l.LatE7, l.LonE7) {
		p.AreaLat = coarseDeg(l.LatE7)
		p.AreaLon = coarseDeg(l.LonE7)
	}
	// Instructions and Brief are published so a job can be priced, but only
	// after being checked. Post refuses entry details in them; this is the
	// backstop on the path that actually reaches an anonymous caller.
	var withheld bool
	for _, f := range []struct {
		src string
		dst *string
	}{
		{l.Instructions, &p.Instructions},
		{l.Brief, &p.Brief},
		{l.Detail, &p.Detail},
		{l.Title, &p.Title},
	} {
		txt, hid := publishable(f.src)
		*f.dst = txt // empty when withheld, so a miss here cannot leak
		if hid {
			withheld = true
		}
	}
	if withheld {
		// Said out loud rather than silently truncated. An operator reading a
		// job with nothing where the instructions should be needs to know it
		// is a redaction and not an empty job.
		p.Withheld = "Some of this job's description is held back until it is " +
			"claimed, because it reads like entry details."
	}
	if l.Pricing == PriceBids {
		// What the buyer would pay is exactly what must not be published.
		return p
	}
	p.PayMinor = l.PayMinor
	p.BonusMinor = l.BonusMinor
	p.AttemptMinor = l.AttemptMinor
	return p
}

// Open reports whether this listing can still be claimed.
func (l *Listing) Open(now time.Time) bool {
	return l.Taken < l.Slots && now.Before(l.Expires)
}

// Pay is what a worker will actually see, in whole units.
func (l *Listing) Pay() string { return minor(l.PayMinor, l.Currency) }

func minor(v int64, cur string) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	sym := "$"
	if strings.ToUpper(cur) != "USD" {
		sym = strings.ToUpper(cur) + " "
	}
	return fmt.Sprintf("%s%s%d.%02d", sign, sym, v/100, v%100)
}

// Board holds open work and hands out the capabilities that claim it.
type Board struct {
	mu       sync.Mutex
	listings map[string]*Listing
	// claims counts seats a client currently holds. It is released when they
	// submit, so it bounds concurrent work rather than work for all time.
	claims map[string]int
	// bids holds offers on open jobs.
	bids map[string][]*Bid
	// projectBids holds offers covering a whole project at once.
	projectBids map[string][]*ProjectBid
	// leases records who holds a seat on each job and until when. A seat with
	// no expiry is a seat somebody can hold forever, which is all it takes to
	// kill a job: claim it, never submit, and the buyer's money sits locked
	// until the listing dies of old age.
	leases map[string]map[string]time.Time
	// rejected counts evidence refused as fabricated, which is not the same
	// as work that failed. See assurance.go.
	rejected map[string]int
	// abandoned counts seats a worker let lapse. Taking work and dropping it
	// costs the buyer a day and the exchange its credibility, so it has to
	// cost the worker something too.
	abandoned map[string]int
	// coolUntil holds a worker out after they abandon, so the cheapest attack
	// — claim, lapse, immediately reclaim — is not a loop.
	coolUntil map[string]time.Time
	// seats records who holds a seat on each job. One client may hold at most
	// one seat on any job: a panel with three seats needs three people, and
	// counting only a global total let one client take all of them.
	seats map[string]map[string]bool
	// completed counts jobs a worker actually submitted for.
	completed map[string]int
	// worked remembers every job a client has ever held a seat on, even after
	// they finish. A ban on judging your own work has to outlive the claim
	// that created the conflict.
	worked map[string]map[string]bool
	// done records which stages each holder has finished, so a long job can
	// report progress and be paid for it piece by piece.
	done map[string]map[string]map[int]bool
	// owner maps a capability's holder hash to the worker it was issued to.
	// The upload path knows only the capability, and the seat it must free is
	// recorded against the worker — without this mapping, finishing a job
	// frees nobody's seat.
	owner map[string]string
	// secrets holds the capability secrets issued per job. Verifying an HMAC
	// requires the secret itself, so it cannot be discarded the way a password
	// hash can — a limitation of the scheme, recorded here rather than hidden.
	secrets map[string][]string
	Caps    *Capabilities
	// Workers lets the board recognise a signed-in operator, so the queue can
	// be the work *they* can take rather than everything in the country.
	Workers *Workers
	Now     func() time.Time
	// MaxClaimsPerClient bounds how much work one claimant may hold at once.
	// It is a ceiling, not an allowance: see allowanceFor.
	MaxClaimsPerClient int
	// ClaimTTL is how long somebody may hold a seat without submitting.
	// Generous enough to walk somewhere; short enough that a griefer costs a
	// buyer an hour rather than a day.
	ClaimTTL time.Duration
	// Cooldown is how long a worker waits after abandoning a seat.
	Cooldown time.Duration
	// Suppliers lets the board attribute work to a business rather than to
	// whichever employee happened to take it, and lets a vetted business hold
	// more at once than a stranger can.
	Suppliers *Suppliers
	// Capacity is what each operator said they would take. Consulted on every
	// claim and assignment, because a preference the dispatcher ignores is
	// worse than no preference at all.
	Capacities *Capacities
	// Funded is consulted before work is listed. Work that cannot pay must not
	// appear on a board: somebody would do it.
	Funded func(l *Listing) error
	// TTL is how long a claimed capability stays valid.
	TTL time.Duration
	// FeeBP is the exchange's cut in basis points, and PayoutThresholdMinor is
	// what has to accumulate before a transfer is made.
	//
	// Carried here so the board can state them. They were applied at
	// settlement and disclosed nowhere: somebody took a job listed at $12.00,
	// received $11.40, and had to work out why. On a marketplace paying people
	// for labour, that is not a UI gap.
	FeeBP                int64
	PayoutThresholdMinor int64
	// Announce is called when work becomes takeable — newly posted, or freed
	// by a lapsed claim. Hooked here rather than at each posting site so a
	// future fourth way to publish work cannot forget to notify anyone.
	Announce func(*Listing)
}

func NewBoard(caps *Capabilities) *Board {
	return &Board{
		listings:  map[string]*Listing{},
		claims:    map[string]int{},
		seats:     map[string]map[string]bool{},
		leases:    map[string]map[string]time.Time{},
		done:      map[string]map[string]map[int]bool{},
		abandoned: map[string]int{},
		rejected:  map[string]int{},
		completed: map[string]int{},
		coolUntil: map[string]time.Time{},
		owner:     map[string]string{},
		worked:    map[string]map[string]bool{},
		secrets:   map[string][]string{},
		Caps:      caps, Now: time.Now,
		MaxClaimsPerClient: 3,
		ClaimTTL:           45 * time.Minute,
		Cooldown:           30 * time.Minute,
		TTL:                2 * time.Hour,
	}
}

func (b *Board) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// Post puts work on the board.
func (b *Board) Post(l *Listing) (err error) {
	defer func() {
		if err == nil && b.Announce != nil {
			go b.Announce(l)
		}
	}()
	if l.Job == "" || l.Title == "" {
		return fmt.Errorf("board: a listing needs a job and a title")
	}
	if err := l.ValidateStages(); err != nil {
		return err
	}
	if err := l.ValidateBrief(); err != nil {
		return err
	}
	// A job too big to settle on one photograph has to say how it settles.
	if err := RequireStaging(l); err != nil {
		return err
	}
	if err := l.ValidateReferences(); err != nil {
		return err
	}
	switch l.Kind {
	case KindObserve, KindDo, KindReview:
	default:
		return fmt.Errorf("board: unknown kind %q", l.Kind)
	}
	if l.Kind == KindDo && l.Instructions == "" {
		return fmt.Errorf("board: a job that asks somebody to do something must say what")
	}
	if l.ExpenseCapMinor < 0 || l.AttemptMinor < 0 {
		return fmt.Errorf("board: amounts must not be negative")
	}
	if l.AttemptMinor > l.PayMinor {
		return fmt.Errorf("board: a failed attempt cannot pay more than finishing")
	}
	// A lease is how long one person may keep a job off the board. The buyer
	// sets it because they know the work; it is capped because a claim is
	// also the cheapest way to make a job disappear. Left uncapped, a
	// 720-hour job was held for a month by whoever clicked first, and the
	// buyer could not cancel it while it was held.
	if l.WorkHours > MaxWorkHours {
		return fmt.Errorf(
			"board: work_hours is %d; the most one person may hold a job is %d "+
				"hours (%d days). Cut longer work into stages, which each carry "+
				"their own lease and are each paid as they are done",
			l.WorkHours, MaxWorkHours, MaxWorkHours/24)
	}
	if l.Slots <= 0 {
		l.Slots = 1
	}
	if l.Currency == "" {
		l.Currency = "USD"
	}
	if l.Posted.IsZero() {
		l.Posted = b.now()
	}
	// Nothing reaches the board that cannot pay for itself. A worker who
	// completes unfunded work has been defrauded by us, not by a counterparty.
	if b.Funded != nil {
		if err := b.Funded(l); err != nil {
			return fmt.Errorf("board: %s cannot be listed: %w", l.Job, err)
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.listings[l.Job] = l
	return nil
}

// Listings returns the open work, soonest to expire first.
func (b *Board) Listings() []*Listing {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	var out []*Listing
	for _, l := range b.listings {
		if l.Open(now) {
			cp := *l
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Expires.Before(out[j].Expires) })
	return out
}

// ForOperator is the board as one operator sees it: only work they could
// actually take, annotated with how far away it is.
//
// Showing everything and letting people discover the refusal on click is the
// arrangement that wastes their time and ours. Filtering here means the queue
// is a list of things they can do.
func (b *Board) ForOperator(worker string, cap Capacity) []*Listing {
	all := b.Listings()
	out := make([]*Listing, 0, len(all))
	for _, l := range all {
		if !IsWork(l.Kind) {
			continue
		}
		if l.Directed() && !l.DirectedAt(b.accountFor(worker)) {
			continue
		}
		if l.Requires != nil && b.Suppliers != nil {
			sup, _ := b.Suppliers.SupplierFor(worker)
			if ok, _ := l.Requires.Met(sup, b.now()); !ok {
				continue
			}
		}
		if !cap.Accepting || !cap.Takes(l.Kind) {
			continue
		}
		if !InRange(l.LatE7, l.LonE7, cap.LatE7, cap.LonE7, cap.RangeMiles) {
			continue
		}
		if !MeetsSkills(l.Skills, cap.Skills) {
			continue
		}
		if len(b.unlicensedFor(worker, l.Skills, b.now())) > 0 {
			continue
		}
		p := l.Public()
		// What this job is a piece of. Without it the board shows three
		// unrelated listings where there is one scope at one address, and an
		// operator prices three mobilisations instead of one.
		if l.ProjectID != "" {
			p.Project = b.briefLocked(l.ProjectID, l.Job)
		}
		p.BlockedBy = b.blockedLocked(l)
		if HasPosition(l.LatE7, l.LonE7) && cap.Positioned() {
			p.DistanceMiles = round1(MilesBetween(l.LatE7, l.LonE7, cap.LatE7, cap.LonE7))
		}
		out = append(out, p)
	}
	out = withoutPracticeIfReal(out)
	// Nearest first: the whole reason somebody shared a location.
	sort.Slice(out, func(i, j int) bool {
		a, bb := out[i].DistanceMiles, out[j].DistanceMiles
		if a == 0 || bb == 0 {
			return out[i].Expires.Before(out[j].Expires)
		}
		return a < bb
	})
	return out
}

func round1(f float64) float64 { return float64(int64(f*10+.5)) / 10 }

// withoutPracticeIfReal hides practice runs once there is real work to show.
//
// A practice job exists so a brand-new operator with an empty board can
// rehearse the flow. Next to paid work it is noise: somebody with three real
// jobs in range does not need to photograph a code on their kitchen table, and
// a board that leads with unpaid rehearsal reads as a board with nothing on it.
func withoutPracticeIfReal(in []*Listing) []*Listing {
	real := false
	for _, l := range in {
		if !l.Practice {
			real = true
			break
		}
	}
	if !real {
		return in
	}
	out := in[:0]
	for _, l := range in {
		if !l.Practice {
			out = append(out, l)
		}
	}
	return out
}

// Widen enlarges the area a job's evidence may come from. It reports whether
// the job was open to be widened, and never shrinks one.
//
// Used when work has sat unfilled: a slightly larger circle is cheaper than a
// repost, which would look like new demand and reset every clock on the job.
func (b *Board) Widen(job string, radiusM int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.listings[job]
	if !ok || !l.Open(b.now()) || radiusM <= l.RadiusM {
		return false
	}
	l.RadiusM = radiusM
	return true
}

// coarseDeg rounds a stored position to two decimal places of a degree —
// roughly a kilometre — which is the only precision the open board publishes.
func coarseDeg(e7 int64) float64 { return math.Round(Deg(e7)*100) / 100 }

// Get returns a listing whether or not it is still open.
func (b *Board) Get(job string) (*Listing, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.listings[job]
	if !ok {
		return nil, false
	}
	cp := *l
	return &cp, true
}

// ErrUnavailable is returned when work cannot be claimed. It says nothing
// about why, so probing the board tells a caller no more than looking at it.
var ErrUnavailable = fmt.Errorf("board: that work is not available")

// Claim takes one slot and mints the capability that does the work.
//
// The capability is scoped to this job and to the actions this kind of work
// needs — a review claim cannot submit evidence, and a task claim cannot cast
// a review. That is enforced by the same middleware that guards every other
// capability route, so the board adds no new trust.
func (b *Board) Claim(job, client string) (secret string, l *Listing, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	b.expireLapsed(now)

	// Two different things are being counted, and conflating them breaks one
	// of them either way.
	//
	// The *account* carries concurrency, cooldown and standing: twelve crews
	// working for one firm are one business holding twelve jobs, not twelve
	// strangers each starting from one, and the firm's record must survive a
	// technician leaving.
	//
	// The *person* carries the seat, the lease and the capability: the buyer
	// needs to know which crew came, two crews may take two slots of the same
	// job, and the technician's own console has to show what they are holding.
	acct := b.accountFor(client)

	item, ok := b.listings[job]
	if !ok || !item.Open(now) {
		return "", nil, ErrUnavailable
	}
	// Directed work belongs to the vendor it was directed to.
	if item.Directed() && !item.DirectedAt(b.accountFor(client)) {
		// The same answer as a job that does not exist. Telling a stranger
		// that this buyer has a vendor, and that it is not them, is telling
		// them about a commercial relationship that is none of their business.
		return "", nil, ErrUnavailable
	}
	// Work that cannot start yet, because something else has to happen first.
	//
	// Checked here rather than only shown on the board, or the order is
	// decorative: two operators read the same listing, both take it, and one
	// of them drives to a site where the ground is still wet. The dependency
	// names the blocking job because the operator is very likely bidding on
	// that one too.
	if len(item.DependsOn) > 0 {
		var waiting []string
		for _, dep := range item.DependsOn {
			if d, ok := b.listings[dep]; ok && !d.Accepted {
				waiting = append(waiting, d.Title)
			}
		}
		if len(waiting) > 0 {
			return "", nil, fmt.Errorf(
				"board: this cannot start until %s is finished and accepted",
				strings.Join(waiting, " and "))
		}
	}
	// A job whose schedule the supplier writes cannot start until that
	// schedule is agreed. Otherwise the crew works an unstaged job, is paid
	// once at the end, and the staging that was the whole point never happens.
	if item.PlanBy == PlanBySupplier && item.PlanState != PlanAccepted {
		if item.PlanState == PlanProposed {
			return "", nil, fmt.Errorf(
				"board: your plan is with the buyer; work can start once they accept it")
		}
		return "", nil, fmt.Errorf(
			"board: this job needs a stage plan from you before it can start")
	}
	// What this account has earned the right to have riding on unfinished work.
	//
	// Checked after the more specific refusals, so somebody blocked by a
	// dependency is told that rather than told about their limit, because it is the only moment that
	// matters: after this the buyer's money is committed and somebody is on
	// their way. A new account taking one expensive job and vanishing is the
	// whole attack, and no amount of looking at the photograph afterwards
	// addresses it. See assurance.go.
	if err := CheckExposure(b.standingLocked(client),
		b.exposureLocked(client), item.AtRiskMinor()); err != nil {
		return "", nil, fmt.Errorf("board: %w", err)
	}
	// What the buyer insists on before anybody sets foot on their property.
	if item.Requires != nil && b.Suppliers != nil {
		sup, _ := b.Suppliers.SupplierFor(client)
		if ok, why := item.Requires.Met(sup, now); !ok {
			return "", nil, fmt.Errorf("board: %s", why)
		}
	}
	if until, ok := b.coolUntil[b.accountFor(client)]; ok && now.Before(until) {
		return "", nil, fmt.Errorf(
			"board: you left work unfinished; you can take more in %d minutes",
			int(until.Sub(now).Minutes())+1)
	}
	// A reviewer must not choose which evidence they judge. Somebody who can
	// choose will eventually choose their own work, or a confederate's, and no
	// conflict rule catches a conflict it cannot see. Reviews come through
	// AssignReview instead.
	if item.Kind == KindReview {
		return "", nil, fmt.Errorf("board: review seats are assigned, not chosen")
	}
	// One seat per client per job. Without this a single claimant takes every
	// seat on a panel and alone decides a finding that settles real money —
	// in either direction, since a buyer can capture a panel to get a refund
	// just as a provider can capture it to get paid.
	if b.seats[job][client] {
		return "", nil, fmt.Errorf("board: you already hold a seat on this one")
	}
	// Nobody judges their own work.
	if item.Parent != "" && b.worked[client][item.Parent] {
		return "", nil, fmt.Errorf("board: you worked on what this reviews")
	}
	// And nobody works on what they have already judged.
	if b.judged(client, job) {
		return "", nil, fmt.Errorf("board: you reviewed work on this one")
	}
	if b.Capacities != nil {
		cap := b.Capacities.Get(client)
		if !cap.Accepting {
			return "", nil, fmt.Errorf("board: you have paused taking work")
		}
		if !cap.Takes(item.Kind) {
			return "", nil, fmt.Errorf("board: you do not take %s work", item.Kind)
		}
		if cap.MaxConcurrent > 0 && b.claims[acct] >= cap.MaxConcurrent {
			return "", nil, fmt.Errorf(
				"board: you set a limit of %d at once; finish one or raise it",
				cap.MaxConcurrent)
		}
		if miss := MissingSkills(item.Skills, cap.Skills); len(miss) > 0 {
			return "", nil, fmt.Errorf(
				"board: that job needs %s; add it in your capacity settings first",
				SkillPhrase(miss))
		}
		// A licensed trade needs a licence somebody checked, not a ticked box.
		//
		// Until now the Licensed flag was decorative: a contractor carrying a
		// state licence, bonding and insurance competed on identical footing
		// with anyone who ticked HVAC, and was underbid by them on sealed-bid
		// licensed work — because the underbidder carried none of the cost.
		// A qualification that costs nothing to claim is worse than none: it
		// looks like a guarantee.
		if unlicensed := b.unlicensedFor(client, item.Skills, now); len(unlicensed) > 0 {
			return "", nil, fmt.Errorf(
				"board: %s requires a licence verified by the exchange; "+
					"add yours under your supplier profile",
				SkillPhrase(unlicensed))
		}
		if !InRange(item.LatE7, item.LonE7, cap.LatE7, cap.LonE7, cap.RangeMiles) {
			return "", nil, fmt.Errorf(
				"board: that is %.0f miles away and you set a %d mile range",
				MilesBetween(item.LatE7, item.LonE7, cap.LatE7, cap.LonE7), cap.RangeMiles)
		}
	}
	if allow := b.allowanceFor(acct); allow > 0 && b.claims[acct] >= allow {
		return "", nil, fmt.Errorf("board: you already hold %d pieces of work; finish one first",
			b.claims[acct])
	}

	actions := []string{ActionView, ActionReview}
	if IsWork(item.Kind) {
		actions = []string{ActionView, ActionSubmit}
	}
	secret, _, err = b.Caps.Issue(job, item.Title, actions, b.TTL)
	if err != nil {
		return "", nil, err
	}
	b.secrets[job] = append(b.secrets[job], secret)
	b.owner[holderOf(secret)] = client
	b.bindWorker(secret, client)
	item.Taken++
	b.claims[acct]++
	if b.seats[job] == nil {
		b.seats[job] = map[string]bool{}
	}
	b.seats[job][client] = true
	if b.leases[job] == nil {
		b.leases[job] = map[string]time.Time{}
	}
	b.leases[job][client] = now.Add(item.LeaseFor(b.claimTTL()))
	if b.worked[client] == nil {
		b.worked[client] = map[string]bool{}
	}
	b.worked[client][job] = true

	cp := *item
	return secret, &cp, nil
}

// expireLapsed returns seats nobody used. Callers hold the lock.
//
// This is what makes a claim a lease rather than a deed. Without it every
// claim is permanent and one free email address can take a job off the board
// for its entire life.
func (b *Board) expireLapsed(now time.Time) {
	for job, holders := range b.leases {
		for worker, until := range holders {
			if now.Before(until) {
				continue
			}
			delete(holders, worker)
			delete(b.seats[job], worker)
			if l, ok := b.listings[job]; ok && l.Taken > 0 {
				l.Taken-- // back on the board for somebody who will do it
			}
			// Counters are keyed to the account the claim was attributed to.
			// Decrementing the person here while Claim incremented their
			// employer would leak a seat on every lapse: the company's
			// concurrency would climb until it could take nothing at all.
			acct := b.accountFor(worker)
			if b.claims[acct] > 0 {
				b.claims[acct]--
			}
			b.abandoned[acct]++
			b.coolUntil[acct] = now.Add(b.cooldown())
		}
	}
}

func (b *Board) claimTTL() time.Duration {
	if b.ClaimTTL > 0 {
		return b.ClaimTTL
	}
	return 45 * time.Minute
}

func (b *Board) cooldown() time.Duration {
	if b.Cooldown > 0 {
		return b.Cooldown
	}
	return 30 * time.Minute
}

// unlicensedFor lists the licensed skills a job needs that this claimant
// cannot show a verified licence for.
//
// Unlicensed skills are unaffected: a ladder is a ladder.
func (b *Board) unlicensedFor(person string, needs []Skill, now time.Time) []Skill {
	var out []Skill
	for _, s := range needs {
		if !Licensed(s) {
			continue
		}
		if b.Suppliers == nil || !b.Suppliers.HoldsLicence(person, s, now) {
			out = append(out, s)
		}
	}
	return out
}

// accountFor is who a claim is attributed to: the supplier when there is one,
// otherwise the person themselves.
func (b *Board) accountFor(person string) string {
	if b.Suppliers == nil {
		return person
	}
	return b.Suppliers.AccountFor(person)
}

// allowanceFor is how many seats this worker may hold at once.
//
// Everybody starts at one. Holding several jobs simultaneously is a privilege
// earned by finishing them, because the cost of a newcomer abandoning three
// seats is borne by three buyers who have no idea who they were dealing with.
// Somebody who abandons repeatedly drops back to one and stays there.
func (b *Board) allowanceFor(worker string) int {
	max := b.MaxClaimsPerClient
	if max <= 0 {
		return 0 // unlimited
	}
	done := b.completed[worker]
	lapsed := b.abandoned[worker]

	// A vetted business is not a stranger, and throttling it like one was the
	// single thing that made this exchange unusable by the supply it most
	// needs. Somebody with twelve crews cannot run twelve crews through a
	// ceiling of three, and no amount of completing jobs one at a time gets
	// them there — the ladder simply stopped.
	//
	// Vetting is a human at the exchange checking licences and cover. It is
	// not self-service, which is what lets it carry this much weight.
	if b.Suppliers != nil {
		if sup, ok := b.Suppliers.SupplierFor(worker); ok && sup.Vetted {
			if lapsed >= 3 {
				// Vetted or not, a supplier dropping work repeatedly gets
				// throttled. Being a real company is not a licence to strand
				// buyers.
				return VettedFloor
			}
			if done >= 5 {
				return VettedMax
			}
			return VettedStart
		}
	}

	switch {
	case lapsed >= 3:
		return 1
	case done >= 5 && lapsed == 0:
		return max
	case done >= 2:
		if max > 2 {
			return 2
		}
		return max
	default:
		return 1
	}
}

// What a vetted supplier may hold at once.
//
// Higher than an individual because a business has crews, and bounded because
// a vetted business can still have a bad week. The floor is what a supplier
// falls back to after repeatedly abandoning work — above an individual's, but
// well below their own ceiling, so the cost of dropping jobs is real.
const (
	VettedStart = 12
	VettedMax   = 40
	VettedFloor = 3
)

// AssignReview seats a worker on a panel the exchange chooses.
//
// The choice is the exchange's, which is the entire point, but it is not
// arbitrary: among the panels this worker is eligible for it takes the one
// closest to expiring, so seats fill in the order they will otherwise be lost.
// Eligibility is every conflict rule that applies to a claim, checked here
// rather than trusted to the caller.
func (b *Board) AssignReview(worker string) (secret string, l *Listing, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	b.expireLapsed(now)
	if until, ok := b.coolUntil[worker]; ok && now.Before(until) {
		return "", nil, fmt.Errorf(
			"board: you left work unfinished; you can take more in %d minutes",
			int(until.Sub(now).Minutes())+1)
	}
	if allow := b.allowanceFor(worker); allow > 0 && b.claims[worker] >= allow {
		return "", nil, fmt.Errorf("board: you already hold %d pieces of work; finish one first",
			b.claims[worker])
	}

	var best *Listing
	for _, item := range b.listings {
		if item.Kind != KindReview || !item.Open(now) {
			continue
		}
		if b.seats[item.Job][worker] {
			continue // already sitting on this panel
		}
		if item.Parent != "" && b.worked[worker][item.Parent] {
			continue // would be judging their own work
		}
		if b.Capacities != nil {
			cap := b.Capacities.Get(worker)
			if !cap.Accepting || !cap.Takes(item.Kind) {
				continue
			}
			if cap.MaxConcurrent > 0 && b.claims[worker] >= cap.MaxConcurrent {
				continue
			}
			if miss := MissingSkills(item.Skills, cap.Skills); len(miss) > 0 {
				return "", nil, fmt.Errorf(
					"board: that job needs %s; add it in your capacity settings first",
					SkillPhrase(miss))
			}
			if !InRange(item.LatE7, item.LonE7, cap.LatE7, cap.LonE7, cap.RangeMiles) {
				continue
			}
		}
		if best == nil || item.Expires.Before(best.Expires) {
			best = item
		}
	}
	if best == nil {
		return "", nil, fmt.Errorf("board: nothing to review right now")
	}

	secret, _, err = b.Caps.Issue(best.Job, best.Title,
		[]string{ActionView, ActionReview}, b.TTL)
	if err != nil {
		return "", nil, err
	}
	b.secrets[best.Job] = append(b.secrets[best.Job], secret)
	b.owner[holderOf(secret)] = worker
	b.bindWorker(secret, worker)
	best.Taken++
	b.claims[worker]++
	if b.seats[best.Job] == nil {
		b.seats[best.Job] = map[string]bool{}
	}
	b.seats[best.Job][worker] = true
	if b.leases[best.Job] == nil {
		b.leases[best.Job] = map[string]time.Time{}
	}
	b.leases[best.Job][worker] = now.Add(b.claimTTL())
	if b.worked[worker] == nil {
		b.worked[worker] = map[string]bool{}
	}
	b.worked[worker][best.Job] = true

	cp := *best
	return secret, &cp, nil
}

// ReviewsWaiting counts panels with seats left, for the board page. The page
// shows a count rather than a list: which panel a reviewer gets is not theirs
// to know before they are seated.
func (b *Board) ReviewsWaiting() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	now := b.now()
	for _, l := range b.listings {
		if l.Kind == KindReview && l.Open(now) {
			n++
		}
	}
	return n
}

// Release gives a seat back when a claimant abandons work.
//
// The client stays recorded in worked: abandoning a task must not restore
// eligibility to review it, or conflicts could be laundered by claiming and
// dropping.
func (b *Board) Release(job, client string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if l, ok := b.listings[job]; ok && l.Taken > 0 {
		l.Taken--
	}
	delete(b.seats[job], client)
	// The seat is the person's; the count is their account's.
	acct := b.accountFor(client)
	if b.claims[acct] > 0 {
		b.claims[acct]--
	}
}

// judged reports whether this client has reviewed a panel whose parent is the
// given job.
func (b *Board) judged(client, job string) bool {
	for reviewed := range b.worked[client] {
		if l, ok := b.listings[reviewed]; ok && l.Parent == job {
			return true
		}
	}
	return false
}

// Secrets returns the capability secrets issued for a job, which capability
// authentication needs in order to recompute an HMAC.
func (b *Board) Secrets(job string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.secrets[job]))
	copy(out, b.secrets[job])
	return out
}

// bindWorker carries an enrolled worker's identity onto the capability it was
// just issued.
//
// Enrolment used to be per-capability: every link asked the phone to make a
// key. A worker enrols once now, so the capability has to inherit what the
// worker already proved — otherwise every submission looks anonymous and
// nothing is ever payable.
func (b *Board) bindWorker(secret, worker string) {
	if strings.HasPrefix(worker, AnonPrefix) {
		return
	}
	_ = b.Caps.Enroll(secret, worker)
}

// holderOf is how a capability names itself: the hash of its secret.
func holderOf(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// WorkerFor returns the worker a capability was issued to.
func (b *Board) WorkerFor(capHolder string) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	w, ok := b.owner[capHolder]
	return w, ok
}

// Done frees a claimant's seat once their work is submitted.
//
// The seat is not returned to the listing — the work was done, not abandoned —
// but the client's concurrent-work count drops, so finishing a job lets them
// start another. Without this the limit is a lifetime cap and an honest worker
// is locked out after three jobs, forever.
//
// A practice run frees the seat and counts for nothing. It exists so somebody
// can learn where the buttons are, and two photographs of a code on a kitchen
// table must not buy the allowance and the ceiling that real completions do.
func (b *Board) Done(job, client string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	counts := true
	if l, ok := b.listings[job]; ok && l.Practice {
		counts = false
	}
	b.finishLocked(job, client, counts)
}

// DoneUnscored frees a seat without recording a completion.
//
// For work that was real enough to hold a seat and not real enough to be a
// record: the demonstration review panel, which is a real page and a real
// flow with nothing behind it.
func (b *Board) DoneUnscored(job, client string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.finishLocked(job, client, false)
}

func (b *Board) finishLocked(job, client string, counts bool) {
	// The lease is the person's; the counters are the account's.
	delete(b.leases[job], client)
	acct := b.accountFor(client)
	if b.claims[acct] > 0 {
		b.claims[acct]--
	}
	if counts {
		b.completed[acct]++
	}
}

// AttachReference adds a buyer's reference image to a listing.
func (b *Board) AttachReference(job string, ref Reference) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.listings[job]
	if !ok {
		return ErrUnavailable
	}
	probe := *l
	probe.References = append(append([]Reference(nil), l.References...), ref)
	if err := probe.ValidateReferences(); err != nil {
		return err
	}
	l.References = probe.References
	return nil
}

// Accept records that a job's work passed verification, which is what
// releases anything waiting on it.
func (b *Board) Accept(job string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if l, ok := b.listings[job]; ok {
		l.Accepted = true
	}
}

// ExpireLapsedClaims returns seats nobody used, and reports how many. Called
// on a timer, because nothing else looks at a claim once it is made.
func (b *Board) ExpireLapsedClaims() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	before := 0
	for _, h := range b.leases {
		before += len(h)
	}
	b.expireLapsed(b.now())
	after := 0
	for _, h := range b.leases {
		after += len(h)
	}
	freed := before - after
	if freed > 0 && b.Announce != nil {
		// A lapsed claim puts real work back in play. Without this, a job
		// abandoned by the first taker sits unannounced until it expires,
		// which is exactly the case where the buyer most needs someone else.
		open := make([]*Listing, 0, freed)
		for job, l := range b.listings {
			slots := l.Slots
			if slots < 1 {
				slots = 1
			}
			if l.Expires.After(b.now()) && len(b.seats[job]) < slots {
				open = append(open, l)
			}
		}
		go func() {
			for _, l := range open {
				b.Announce(l)
			}
		}()
	}
	return freed
}

// Holding is work this person currently has out.
type Holding struct {
	Job     string    `json:"job"`
	Kind    string    `json:"kind"`
	Title   string    `json:"title"`
	Where   string    `json:"where,omitempty"`
	Expires time.Time `json:"expires"`
	// Resume is the link back into the work, capability and all.
	Resume string `json:"resume"`
	// PayMinor is what the whole job pays, and Currency what in.
	PayMinor int64  `json:"pay_minor,omitempty"`
	Currency string `json:"currency,omitempty"`
	// Stages is the plan, so the page can show which piece is in front of
	// them rather than one undifferentiated "in flight". A person part-way
	// through a three-day job wants to know what is next and what it pays.
	Stages []Stage `json:"stages,omitempty"`
	// StageDone marks the pieces already evidenced and paid.
	StageDone []bool `json:"stage_done,omitempty"`
	// NextStage is the index of the piece in front of them, or -1 when the
	// whole job is finished.
	NextStage int `json:"next_stage"`
	// BlockedBy names anything that has to happen before this can start.
	BlockedBy []string `json:"blocked_by,omitempty"`
	// Agreed is what the winning bid said it priced on, carried so the crew
	// can read the figures the work is judged against.
	Agreed []Assumption `json:"agreed,omitempty"`
	// Project is the wider scope this belongs to, if any.
	Project *ProjectBrief `json:"project,omitempty"`
}

// HeldBy returns the work a person is holding, with a way back into each.
//
// Without this a claim is a one-way door: the capability lives in a URL
// fragment, so closing that tab loses the work entirely, and the lease keeps
// the worker locked out of taking anything else until it lapses. They are told
// they already hold a job and given no way to reach it.
func (b *Board) HeldBy(worker string) []Holding {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expireLapsed(b.now())

	var out []Holding
	for job, holders := range b.seats {
		if !holders[worker] {
			continue
		}
		l, ok := b.listings[job]
		if !ok {
			continue
		}
		// Find the secret issued to this worker for this job.
		var secret string
		for _, cand := range b.secrets[job] {
			if b.owner[holderOf(cand)] == worker {
				secret = cand
				break
			}
		}
		if secret == "" {
			continue
		}
		page := "/w/"
		if l.Kind == KindReview {
			page = "/r/"
		}
		h := Holding{
			Job: job, Kind: l.Kind, Title: l.Title, Where: l.Where,
			Expires:  b.leases[job][worker],
			Resume:   page + job + "#" + secret,
			PayMinor: l.PayMinor, Currency: l.Currency,
			Stages: l.Stages, Agreed: l.Agreed,
			NextStage: -1,
			BlockedBy: b.blockedLocked(l),
		}
		if l.Staged() {
			done := b.done[job][worker]
			h.StageDone = make([]bool, len(l.Stages))
			for i := range l.Stages {
				h.StageDone[i] = done[i]
				if !done[i] && h.NextStage < 0 {
					h.NextStage = i
				}
			}
		} else {
			h.NextStage = 0
		}
		if l.ProjectID != "" {
			h.Project = b.briefLocked(l.ProjectID, job)
		}
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Expires.Before(out[j].Expires) })
	return out
}

// GiveBack releases a seat the worker no longer wants.
//
// Deliberately handing work back is not abandonment: the job returns to the
// board immediately instead of sitting dead until the lease lapses, so the
// honest thing to do is also the cheap one. No cooldown, no mark against them.
func (b *Board) GiveBack(job, worker string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.seats[job][worker] {
		return fmt.Errorf("board: you are not holding that")
	}
	delete(b.seats[job], worker)
	delete(b.leases[job], worker)
	if l, ok := b.listings[job]; ok && l.Taken > 0 {
		l.Taken--
	}
	// Same account the claim was counted against, for the same reason.
	acct := b.accountFor(worker)
	if b.claims[acct] > 0 {
		b.claims[acct]--
	}
	return nil
}

// Standing is what a worker has earned, for the console.
//
// Reported for the account the work is attributed to, so an employee sees
// their employer's record rather than an empty one — it is the record that
// governs what they can take.
func (b *Board) Standing(worker string) (completed, abandoned, allowance int, coolUntil time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	acct := b.accountFor(worker)
	return b.completed[acct], b.abandoned[acct],
		b.allowanceFor(acct), b.coolUntil[acct]
}

// RegisterBoard mounts the marketplace.
func (b *Board) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /board", b.handlePage)
	mux.HandleFunc("GET /v1/board", b.handleList)
	// Claiming happens through /v1/workers/claim/{job}, which requires a
	// worker identity. There is deliberately no anonymous claim route: every
	// rule that bounds abuse keys on who is asking.
}

func (b *Board) handleList(w http.ResponseWriter, r *http.Request) {
	// Review panels are summarised, not listed. Publishing which panels are
	// open, and what each is about, would let somebody wait for a particular
	// one to come up and take it — which is choosing what to judge by another
	// route.
	// Signed in, and we know where they are: show the work they can actually
	// take, nearest first. This is also the polling endpoint an API operator
	// uses — no separate route, because "what work is there for me" is the
	// same question whether a person or a program is asking.
	var (
		tasks        []*Listing
		personalized bool
		filteredAway int
	)
	if b.Workers != nil {
		body, _ := readBody(r)
		if worker, err := b.Workers.Authenticate(r, body, b.now()); err == nil {
			cap := b.Capacities.Get(worker.ID)
			tasks = b.ForOperator(worker.ID, cap)
			personalized = true
			for _, l := range b.Listings() {
				if IsWork(l.Kind) {
					filteredAway++
				}
			}
			filteredAway -= len(tasks)
		}
	}
	if !personalized {
		for _, l := range b.Listings() {
			// Directed work is not a market event and does not belong on a
			// public board: it is already assigned.
			if IsWork(l.Kind) && !l.Directed() {
				p := l.Public()
				if l.ProjectID != "" {
					p.Project = b.BriefFor(l.Job)
				}
				p.BlockedBy = b.Blocked(l.Job)
				tasks = append(tasks, p)
			}
		}
		tasks = withoutPracticeIfReal(tasks)
	}
	if tasks == nil {
		tasks = []*Listing{}
	}
	out := map[string]any{
		"work":            tasks,
		"reviews_waiting": b.ReviewsWaiting(),
		"personalized":    personalized,
		"terms": map[string]any{
			"fee_bp":                 b.FeeBP,
			"payout_threshold_minor": b.PayoutThresholdMinor,
			"currency":               "USD",
		},
	}
	if personalized && filteredAway > 0 {
		// Say what was hidden and why. A filtered queue that silently shrinks
		// looks like an empty marketplace, and the operator's fix — widen the
		// range, add a skill — is invisible unless we name it.
		out["filtered_out"] = filteredAway
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// clientOf identifies a claimant well enough to rate-limit them.
//
// It is an address, not an identity, and it is not a defence against a
// determined sybil — that needs real accounts, which is noted as unsolved
// rather than papered over here.
func clientOf(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.IndexByte(fwd, ','); i > 0 {
			return strings.TrimSpace(fwd[:i])
		}
		return strings.TrimSpace(fwd)
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	return host
}

func (b *Board) handlePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	fmt.Fprint(w, boardPageHTML)
}

// HasOpenSeat reports whether anybody could still take this job.
//
// Dispatch checks it between offers so a full job stops being announced. An
// operator who is offered work that is already gone learns to ignore the
// channel, which costs more than the missed dispatch.
func (b *Board) HasOpenSeat(job string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	l := b.listings[job]
	if l == nil || l.Expires.Before(b.now()) {
		return false
	}
	slots := l.Slots
	if slots < 1 {
		slots = 1
	}
	return len(b.seats[job]) < slots
}

// Bookable reports whether the work may be done at this moment.
//
// Separate from expiry on purpose: a job can be live on the board for a week
// and only doable during a two-hour window on Tuesday. Claiming outside the
// window is not refused — somebody may reasonably take a job in the morning
// intending to do it that afternoon — but the window travels with the brief so
// nobody turns up at the wrong time.
func (l *Listing) Bookable(now time.Time) bool {
	if !l.NotBefore.IsZero() && now.Before(l.NotBefore) {
		return false
	}
	if !l.NotAfter.IsZero() && now.After(l.NotAfter) {
		return false
	}
	return true
}

// Window renders the booking window the way somebody would say it, or "" when
// the job has none.
func (l *Listing) Window() string {
	switch {
	case l.NotBefore.IsZero() && l.NotAfter.IsZero():
		return ""
	case l.NotBefore.IsZero():
		return "before " + l.NotAfter.Format("Mon 3:04pm")
	case l.NotAfter.IsZero():
		return "after " + l.NotBefore.Format("Mon 3:04pm")
	case l.NotBefore.Format("Mon Jan 2") == l.NotAfter.Format("Mon Jan 2"):
		return l.NotBefore.Format("Mon 3:04pm") + "–" + l.NotAfter.Format("3:04pm")
	default:
		return l.NotBefore.Format("Mon 3:04pm") + " to " + l.NotAfter.Format("Mon 3:04pm")
	}
}

// HoldersOf lists who currently holds seats on a job.
func (b *Board) HoldersOf(job string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for worker := range b.seats[job] {
		out = append(out, worker)
	}
	sort.Strings(out)
	return out
}

// Progress records that a holder finished a stage, and keeps their lease
// alive.
//
// Somebody actively working is the opposite of somebody who has abandoned the
// job, and the lease exists to tell those apart. Before stages there was
// nothing to distinguish them: a crew three hours into a two-day job looked
// exactly like a griefer who claimed and vanished.
func (b *Board) Progress(job, worker string, stage int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.listings[job]
	if !ok {
		return fmt.Errorf("board: no such job")
	}
	if !b.seats[job][worker] {
		return fmt.Errorf("board: you are not holding that")
	}
	if b.done[job] == nil {
		b.done[job] = map[string]map[int]bool{}
	}
	if b.done[job][worker] == nil {
		b.done[job][worker] = map[int]bool{}
	}
	b.done[job][worker][stage] = true

	// Extend rather than reset to a fixed window: the remaining work is what
	// is left, and a job that keeps renewing forever is a job nobody is doing.
	now := b.now()
	if b.leases[job] != nil {
		remaining := l.LeaseFor(b.claimTTL())
		if until, ok := b.leases[job][worker]; ok && until.After(now) {
			// Never shorten a lease by reporting progress.
			if extended := now.Add(remaining); extended.After(until) {
				b.leases[job][worker] = extended
			}
		} else {
			b.leases[job][worker] = now.Add(remaining)
		}
	}
	return nil
}

// StagesDone reports which stages this holder has finished.
func (b *Board) StagesDone(job, worker string) map[int]bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[int]bool{}
	for k, v := range b.done[job][worker] {
		out[k] = v
	}
	return out
}

// NextStage is the stage this holder should be working on, and whether the job
// is finished.
//
// Stages run in order because trade work does: nobody surfaces a driveway
// before the base is in, and allowing the final stage to be claimed first
// would let somebody photograph a finished-looking result and skip everything
// underneath it.
func (b *Board) NextStage(job, worker string) (idx int, s Stage, done bool) {
	b.mu.Lock()
	l, ok := b.listings[job]
	finished := b.done[job][worker]
	b.mu.Unlock()
	if !ok || !l.Staged() {
		return 0, Stage{}, false
	}
	for i, st := range l.Stages {
		if !finished[i] {
			return i, st, false
		}
	}
	return len(l.Stages), Stage{}, true
}

// Cancel withdraws work nobody has taken.
//
// Refuses once somebody holds a seat: the caller checks that too, but the
// board is where the seats are and a race between the two would otherwise
// cancel a job out from under somebody at the address.
func (b *Board) Cancel(job string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.listings[job]
	if !ok {
		return fmt.Errorf("board: no such job")
	}
	if len(b.seats[job]) > 0 {
		return fmt.Errorf("board: somebody is holding that right now")
	}
	if l.Cancelled {
		return nil
	}
	l.Cancelled = true
	// Expire it so nothing else on the board treats it as open.
	l.Expires = b.now()
	return nil
}

// Bid returns one offer on a job.
//
// Needed by the award path, which has to know the agreed amount before it can
// escrow it: an open job holds nothing until somebody's price is accepted.
func (b *Board) Bid(job, bidID string) (*Bid, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, x := range b.bids[job] {
		if x.ID == bidID {
			c := *x
			return &c, true
		}
	}
	return nil, false
}

// Directed reports whether this job is reserved for named suppliers.
func (l *Listing) Directed() bool { return len(l.DirectedTo) > 0 }

// DirectedAt reports whether this account is one of them.
func (l *Listing) DirectedAt(account string) bool {
	for _, s := range l.DirectedTo {
		if s == account {
			return true
		}
	}
	return false
}

// Funding is a card authorisation standing behind a listing.
//
// The gateless path: nobody signed in, nobody topped up. The buyer's card is
// authorised for the ceiling when the job is posted and captured, once, for
// exactly what the job paid out — the rest is released. Money that was never
// captured never needs refunding, which is what makes this path hold less of
// other people's money than the balance path, not more.
type Funding struct {
	// Kind is "card", or "usdc" for a stablecoin transfer received on-chain;
	// a place for "invoice" and friends later.
	Kind string
	// Intent is the authorised, uncaptured PaymentIntent on the rail, or for
	// a chain transfer the transaction hash that funded the job.
	Intent string
	// Payer is the sending address of a chain transfer: where an unspent
	// remainder is owed back to. Empty for a card.
	Payer string
	// Session is the checkout session that produced the authorisation.
	Session string
	// AuthorizedMinor is the ceiling the card was authorised for.
	AuthorizedMinor int64
	// Email is the payer's address as the rail reported it — the only contact
	// the exchange has for a buyer who never made an account.
	Email string
	// Settled records that the card side has been closed: captured for what
	// was paid out, or released. Once.
	Settled bool
}
