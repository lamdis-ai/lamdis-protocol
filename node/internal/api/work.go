package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/media"
)

// WorkServer is the provider's side of the marketplace: the page somebody
// opens after claiming a task, and the endpoint their photograph arrives on.
//
// It is capability-authenticated like the reviewer surface, and for the same
// reason — the person doing the work is a stranger with a phone, not a
// principal with a keypair. The middleware is shared, so a task capability
// reaching a review route is refused by the same code that refuses everything
// else.
type WorkServer struct {
	Caps  *Capabilities
	Board *Board
	// Secrets supplies candidate capability secrets for a job. It defaults to
	// the board's, which is where a worker's capability came from.
	Secrets func(job string) []string
	Replay  *ReplayGuard
	// Submit records accepted evidence. It receives the raw bytes exactly as
	// uploaded, because re-encoding destroys the EXIF the verifier depends on.
	Submit func(Submission) (Submission, error)
	// Store persists one uploaded file. It is separate from Submit because
	// files arrive one at a time and a submission is finalised once.
	Store func(job string, a Artifact, data []byte) error
	Now   func() time.Time

	mu      sync.Mutex
	sent    map[string]bool       // capability holder -> already submitted
	pending map[string][]Artifact // capability holder -> files so far

	// MaxUploadBytes caps a single submission.
	MaxUploadBytes int64
}

func (s *WorkServer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *WorkServer) maxUpload() int64 {
	if s.MaxUploadBytes > 0 {
		return s.MaxUploadBytes
	}
	return 24 << 20 // 24 MiB: a modern phone photograph, with room to spare
}

func (s *WorkServer) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /w/{job}", s.handlePage)
	mux.HandleFunc("GET /v1/work/{job}", s.withCapability(ActionView, s.handleBrief))
	mux.HandleFunc("POST /v1/work/{job}/evidence", s.withCapability(ActionSubmit, s.handleUpload))
	mux.HandleFunc("POST /v1/work/{job}/submit", s.withCapability(ActionSubmit, s.handleFinalize))
}

// withCapability mirrors the reviewer surface's middleware, including the
// signature that hands the handler a *Capability rather than a principal id.
func (s *WorkServer) withCapability(
	action string,
	next func(w http.ResponseWriter, r *http.Request, c *Capability, body []byte),
) http.HandlerFunc {
	src := s.Secrets
	if src == nil && s.Board != nil {
		src = s.Board.Secrets
	}
	rs := &ReviewServer{Caps: s.Caps, Secrets: src, Replay: s.Replay, Now: s.Now}
	return rs.withCapability(action, next)
}

// Artifact is one uploaded file and what was derived from it.
//
// A video contributes more than itself: the frames pulled out of it and the
// transcript of what was said are both evidence, and both are recorded here so
// the trail shows what the verifier actually read rather than only what was
// handed over.
type Artifact struct {
	SHA256 string `json:"sha256"`
	Mime   string `json:"mime"`
	Bytes  int    `json:"bytes"`
	// Kind is image, video or audio.
	Kind string `json:"kind"`
	// Frames are the content hashes of stills taken from a video, in order.
	Frames []string `json:"frames,omitempty"`
	// Transcript is what was audible. Empty for a silent clip or a photograph.
	Transcript string `json:"transcript,omitempty"`
	// DurationMS is how long a video or audio file runs.
	DurationMS int64 `json:"duration_ms,omitempty"`
	// HasGeo, LatE7 and LonE7 are where the file says it was taken. Integer
	// degrees times 1e7, like everything else positional here.
	HasGeo bool  `json:"has_geo,omitempty"`
	LatE7  int64 `json:"lat_e7,omitempty"`
	LonE7  int64 `json:"lon_e7,omitempty"`
	// CapturedAt is the file's own asserted capture time. Attacker-supplied,
	// so weighted rather than trusted.
	CapturedAt time.Time `json:"captured_at,omitempty"`
	// ChallengeSeen records where the code was found: "text" if a describer
	// read it in the picture, "spoken" if it was said aloud, "" if not at all.
	ChallengeSeen string `json:"challenge_seen,omitempty"`
}

// Submission is everything one worker uploaded for one job.
//
// It is a set rather than a single file because one photograph is one claim
// about one moment, and the things buyers actually want established — a sign
// is up, a dock is clear, a room is empty — are better shown by several angles
// or by a continuous clip than by a single frame somebody could have staged.
type Submission struct {
	Job       string     `json:"job"`
	Artifacts []Artifact `json:"artifacts"`
	// Challenge is the code this claimant was told to put in frame or say
	// aloud. The verifier must find it in at least one artifact.
	Challenge string `json:"challenge"`
	// AttestedBy is "device_key" when the claimant proved they hold a keypair,
	// and "capability" when the exchange is vouching for them.
	AttestedBy string    `json:"attested_by"`
	Holder     string    `json:"holder"`
	At         time.Time `json:"at"`
	// Verified is false until something has actually looked at the evidence. A
	// submission is never payable while this is false.
	Verified bool `json:"verified"`
	// LatE7, LonE7 and RadiusM are the area the job named, copied from the
	// listing so a submission carries the standard it was judged against.
	LatE7   int64 `json:"lat_e7,omitempty"`
	LonE7   int64 `json:"lon_e7,omitempty"`
	RadiusM int64 `json:"radius_m,omitempty"`
	// Finding is what the evidence showed, for an observation: true when the
	// predicate holds. Meaningless for a do-job, where the question is whether
	// it was done.
	Finding bool `json:"finding,omitempty"`
	// Attempted marks a do-job the worker went to and could not complete —
	// the shop was shut, the address does not exist — with evidence of having
	// been there. It earns the attempt fee, not the completion fee.
	Attempted bool `json:"attempted,omitempty"`
	// Why explains a refusal in words the worker can act on.
	Why string `json:"why,omitempty"`

	// ExpenseMinor is what the worker laid out and is claiming back, bounded
	// by the job's cap.
	//
	// The cap was escrowed from the beginning and there was no way to claim
	// against it: a buyer set aside five dollars for bin bags and the person
	// who bought them could never ask for it. Money held for somebody with no
	// path to it is worse than money never offered.
	ExpenseMinor int64 `json:"expense_minor,omitempty"`
	// ExpenseNote is what it was spent on, in the worker's words.
	ExpenseNote string `json:"expense_note,omitempty"`

	// Tier is the standard the buyer asked for, copied from the listing so a
	// submission carries what it must reach.
	Tier string `json:"tier,omitempty"`

	// SiteMark is what had to be legible to show this is the right property.
	// Copied from the listing so a submission carries the standard it was
	// judged against, exactly as the geofence is.
	SiteMark *SiteMark `json:"site_mark,omitempty"`
	// MarkSeen records that it was found. False with a mark set means the
	// photographs could be of anywhere.
	MarkSeen bool `json:"mark_seen,omitempty"`

	// Signals are what the describer thought of the imagery, recorded whether
	// or not anything currently acts on them.
	//
	// Only one of these gates a submission today. The others are kept because
	// a threshold cannot be measured without a corpus, and a corpus does not
	// exist unless somebody stores the numbers before they are useful.
	Signals *ImageSignals `json:"signals,omitempty"`

	// Stage is which piece of a longer job this evidences, by position.
	// Meaningless on a single-visit job, where there is only ever one.
	Stage int `json:"stage,omitempty"`
	// StageName is that stage in words, for anybody reading the record later.
	StageName string `json:"stage_name,omitempty"`

	// Report is the worker's written answer to a job that asked for one, as
	// rows of the fields the listing named. See report.go.
	Report []ReportRow `json:"report,omitempty"`
	// Reached is the tier the evidence actually supported, when it is lower
	// than what the job asked for. A report with no photograph is a signed
	// claim and nothing more, and the receipt must say V0 rather than repeat
	// the tier the buyer requested.
	Reached string `json:"reached,omitempty"`
}

// SHA256 is the first artifact's hash, for callers that still think in one
// file. It is a convenience, not the identity of the submission.
func (s Submission) SHA256() string {
	if len(s.Artifacts) == 0 {
		return ""
	}
	return s.Artifacts[0].SHA256
}

// MaxArtifacts bounds one submission. Enough for several angles or a clip and
// a still; not enough to use the exchange as storage.
const MaxArtifacts = 6

// ChallengeFor derives the code a claimant must include in their photograph.
//
// It is per-capability rather than per-listing, so two people working the same
// task get different codes and neither can use the other's picture. It is
// never published on the board: a code a stranger can read without claiming is
// a code they can composite into an old photograph.
func ChallengeFor(job string, c *Capability) string {
	return ChallengeForStage(job, c, 0)
}

// ChallengeForStage derives the code for one stage of a job.
//
// A single code covering every stage meant one photograph of one card tied the
// whole job: the crew could shoot the card once and reuse it for prep, base
// and surface. Each stage now needs its own capture, which is the only thing
// making "evidence of this stage" mean anything.
func ChallengeForStage(job string, c *Capability, stage int) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "lamdis-challenge:%s:%s:%d",
		job, c.Holder, stage))
	return secretAlphabet.EncodeToString(sum[:])[:6]
}

type workBrief struct {
	Job   string `json:"job"`
	Title string `json:"title"`
	// Where is the exact address, released here and nowhere else.
	Where  string `json:"where,omitempty"`
	Detail string `json:"detail,omitempty"`
	// Instructions and Deliverable are what the job actually asks for.
	//
	// Also on the public board now, and they have to be: somebody asked to
	// name a price cannot do it without them. What stays private is Access,
	// below, which is the part that was really the reason to withhold these.
	Instructions string `json:"instructions,omitempty"`
	Deliverable  string `json:"deliverable,omitempty"`
	// Access is how to get in. Released here and nowhere else, alongside Where.
	Access string `json:"access,omitempty"`
	// Brief is the buyer's agent's own text, carried through untouched.
	Brief string `json:"brief,omitempty"`
	// Agreed is what the winning bid said it priced on — the driveway width,
	// the barn footprint. The work is judged against these, so the person
	// doing it has to be able to read them.
	Agreed []Assumption `json:"agreed,omitempty"`
	// Window is when the buyer needs this done, in words. Empty if any time
	// will do.
	Window string `json:"window,omitempty"`
	// Stage is the piece of a longer job to do next: what it is called, where
	// it sits in the run, what would prove it, and what it pays on its own.
	//
	// The crew is judged against this rather than against the finished job,
	// because "the driveway is paved" is not true yet when the base is down
	// and refusing honest work for that would be the system's mistake.
	Stage         string `json:"stage,omitempty"`
	StageOf       string `json:"stage_of,omitempty"`
	StageProves   string `json:"stage_proves,omitempty"`
	StagePayMinor int64  `json:"stage_pay_minor,omitempty"`
	PayMinor      int64  `json:"pay_minor"`
	BonusMinor    int64  `json:"bonus_minor,omitempty"`
	Currency      string `json:"currency"`
	Challenge     string `json:"challenge"`
	Tier          string `json:"tier,omitempty"`
	Expires       string `json:"expires"`
	// Report is the table the job wants filled in, when the deliverable is
	// information rather than a photograph. The page renders a form from it.
	Report []ReportField `json:"report,omitempty"`
	// Practice marks a rehearsal: nothing is paid and nothing is recorded.
	Practice bool `json:"practice,omitempty"`
}

func (s *WorkServer) handleBrief(w http.ResponseWriter, r *http.Request, c *Capability, _ []byte) {
	l, ok := s.Board.Get(c.Job)
	if !ok {
		refuse(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	brief := workBrief{
		Job: l.Job, Title: l.Title, Where: l.Where, Detail: l.Detail,
		Instructions: l.Instructions, Deliverable: l.Deliverable,
		Access: l.Access, Brief: l.Brief, Agreed: l.Agreed,
		Window:   l.Window(),
		PayMinor: l.PayMinor, BonusMinor: l.BonusMinor, Currency: l.Currency,
		Tier:    l.Tier,
		Expires: l.Expires.UTC().Format(time.RFC3339),
		Report:  l.Report, Practice: l.Practice,
	}
	// The code is per stage, so it has to be derived from the stage the crew
	// is actually on.
	briefStage := 0
	if l.Staged() {
		if idx, _, all := s.Board.NextStage(c.Job, s.holderWorker(c)); !all {
			briefStage = idx
		}
	}
	brief.Challenge = ChallengeForStage(c.Job, c, briefStage)
	// On a staged job the crew needs to know which piece they are on, and be
	// judged against that piece rather than the finished job.
	if l.Staged() {
		if idx, st, all := s.Board.NextStage(c.Job, s.holderWorker(c)); !all {
			brief.Stage = st.Name
			brief.StageOf = fmt.Sprintf("%d of %d", idx+1, len(l.Stages))
			brief.StageProves = st.Deliverable
			brief.StagePayMinor = st.PayMinor
			// The deliverable shown is the stage's, not the job's.
			brief.Deliverable = st.Deliverable
		}
	}
	json.NewEncoder(w).Encode(brief)
}

// handleUpload accepts the photograph.
//
// The bytes arrive as the raw request body rather than a multipart form, which
// is deliberate. The capability signature covers a hash of the body, and a
// browser picks its own multipart boundary — so a multipart upload could not
// be signed by the client at all. Sending the file raw means the signature
// binds these exact image bytes, which is a stronger property than the form
// would have given.
//
// They are stored exactly as they arrived. Every transformation between the
// camera and here — a canvas re-encode, a resize, a strip of metadata —
// removes signal the verifier uses to decide whether this picture was taken
// where and when it claims.
// handleUpload accepts one file. A submission may hold several, uploaded one
// at a time.
//
// One file per request rather than a multipart form, deliberately: the
// capability signature covers a hash of the body, a browser picks its own
// multipart boundary, and so a form upload could not be signed by the client
// at all. Sending each file raw means the signature binds those exact bytes.
//
// The bytes are stored as they arrived. Every transformation between the
// camera and here — a re-encode, a resize, a strip of metadata — removes
// signal the verifier uses.
func (s *WorkServer) handleUpload(w http.ResponseWriter, r *http.Request, c *Capability, body []byte) {
	if s.Submit == nil {
		refuse(w)
		return
	}
	if len(body) == 0 {
		writeWork(w, http.StatusBadRequest, map[string]string{"error": "no file was attached"})
		return
	}
	if int64(len(body)) > s.maxUpload() {
		writeWork(w, http.StatusRequestEntityTooLarge, map[string]string{
			"error": "that file is too large"})
		return
	}
	mime := sniff(body)
	kind := media.KindOf(mime)
	if mime == "" || kind == "" {
		writeWork(w, http.StatusBadRequest, map[string]string{
			"error": "only JPEG, PNG, HEIC and MP4 are accepted"})
		return
	}

	s.mu.Lock()
	if s.pending == nil {
		s.pending = map[string][]Artifact{}
	}
	if s.sent[c.Holder] {
		s.mu.Unlock()
		writeWork(w, http.StatusConflict, map[string]string{
			"error": "you have already submitted for this job"})
		return
	}
	if len(s.pending[c.Holder]) >= MaxArtifacts {
		s.mu.Unlock()
		writeWork(w, http.StatusConflict, map[string]string{
			"error": fmt.Sprintf("a submission may hold at most %d files", MaxArtifacts)})
		return
	}
	s.mu.Unlock()

	sum := sha256.Sum256(body)
	art := Artifact{
		SHA256: hex.EncodeToString(sum[:]), Mime: mime,
		Bytes: len(body), Kind: kind,
	}
	if s.Store != nil {
		if err := s.Store(c.Job, art, body); err != nil {
			writeWork(w, http.StatusInternalServerError,
				map[string]string{"error": "could not store that file"})
			return
		}
	}

	s.mu.Lock()
	// The same file twice adds nothing and would be counted twice downstream.
	for _, existing := range s.pending[c.Holder] {
		if existing.SHA256 == art.SHA256 {
			s.mu.Unlock()
			writeWork(w, http.StatusOK, map[string]any{
				"ok": true, "duplicate": true, "sha256": art.SHA256,
				"files": len(s.pending[c.Holder]),
			})
			return
		}
	}
	s.pending[c.Holder] = append(s.pending[c.Holder], art)
	n := len(s.pending[c.Holder])
	s.mu.Unlock()

	writeWork(w, http.StatusOK, map[string]any{
		"ok": true, "sha256": art.SHA256, "bytes": art.Bytes,
		"type": art.Mime, "kind": art.Kind, "files": n,
		"max_files": MaxArtifacts,
	})
}

// handleFinalize closes a submission and sends it to be verified.
//
// Uploading and submitting are separate acts because they mean different
// things: a file on its own is not a claim, and a worker adding a second angle
// has not yet said they are finished.
func (s *WorkServer) handleFinalize(w http.ResponseWriter, r *http.Request, c *Capability, body []byte) {
	if s.Submit == nil {
		refuse(w)
		return
	}
	// Which piece of the job this is. On a single-visit job there is exactly
	// one, and everything below behaves as it always did.
	stageIdx, stage, allDone := s.Board.NextStage(c.Job, s.holderWorker(c))
	l, _ := s.Board.Get(c.Job)
	staged := l != nil && l.Staged()
	if staged && allDone {
		writeWork(w, http.StatusConflict, map[string]string{
			"error": "every stage of this job is already submitted"})
		return
	}

	s.mu.Lock()
	// A staged job is finished a piece at a time, so the block is per stage
	// rather than per job — otherwise a crew could evidence the base course
	// and never be allowed to show the surface.
	key := c.Holder
	if staged {
		key = fmt.Sprintf("%s#%d", c.Holder, stageIdx)
	}
	if s.sent[key] {
		s.mu.Unlock()
		writeWork(w, http.StatusConflict, map[string]string{
			"error": "you have already submitted for this"})
		return
	}
	arts := append([]Artifact(nil), s.pending[c.Holder]...)
	s.mu.Unlock()

	// An expense claim, if they laid anything out. Bounded at settlement by
	// the cap the buyer escrowed; asked for here because the moment they
	// finish is the only moment they have the receipt in hand.
	var claim struct {
		ExpenseMinor int64  `json:"expense_minor"`
		ExpenseNote  string `json:"expense_note"`
		// Attempted is the worker saying they went and could not finish: the
		// shop was shut, the gate was locked, the unit was not the one
		// described. It earns the attempt fee rather than the completion fee,
		// and it still requires evidence of having been there.
		Attempted bool   `json:"attempted"`
		Why       string `json:"why"`
		// Report is the written answer, for a job that asked for one. A
		// photograph alongside it is welcome and optional.
		Report []ReportRow `json:"report"`
	}
	if len(body) > 0 {
		json.Unmarshal(body, &claim)
	}

	// A job that asked for a table takes a table. Anything else needs a
	// photograph, and a report job with neither is told which it is missing.
	var report []ReportRow
	if l != nil && len(l.Report) > 0 && len(claim.Report) > 0 {
		rows, err := ValidateReport(l.Report, claim.Report)
		if err != nil {
			writeWork(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		report = rows
	}
	if len(arts) == 0 && len(report) == 0 {
		msg := "add at least one photo or video first"
		if l != nil && len(l.Report) > 0 {
			msg = "fill in the report first; a photo is optional on this job"
		}
		writeWork(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}

	attested := "capability"
	if c.DevicePrincipal != "" {
		attested = "device_key"
	}
	sub := Submission{
		Job: c.Job, Artifacts: arts,
		Challenge:  ChallengeForStage(c.Job, c, stageIdx),
		AttestedBy: attested, Holder: c.Holder, At: s.now(),
		ExpenseMinor: claim.ExpenseMinor, ExpenseNote: claim.ExpenseNote,
		Attempted: claim.Attempted, Why: claim.Why,
		Stage: stageIdx, StageName: stage.Name,
		Report: report,
	}
	if l != nil {
		sub.Tier = l.Tier
		if l.TiedToPlace() {
			sub.SiteMark = l.MarkFor()
		}
		// The area the job named, so the evidence can be checked against it.
		//
		// These were declared on Submission and never populated, so
		// sub.RadiusM was always zero and checkGeofence never ran — for any
		// job, ever. The fence existed, was tested, and was unreachable.
		sub.LatE7, sub.LonE7, sub.RadiusM = l.LatE7, l.LonE7, l.RadiusM
	}
	stored, err := s.Submit(sub)
	if err != nil {
		// A refusal is not a submission spent: the worker can add a better
		// file and try again, which is the whole reason to say why.
		writeWork(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	s.mu.Lock()
	if s.sent == nil {
		s.sent = map[string]bool{}
	}
	s.sent[key] = true
	delete(s.pending, c.Holder)
	s.mu.Unlock()
	if s.Board != nil {
		if worker, ok := s.Board.WorkerFor(c.Holder); ok {
			if staged && stored.Verified {
				// Finishing a stage is progress, not completion. Releasing the
				// seat here would put a half-paved driveway back on the board
				// and hand the rest of the job to a stranger.
				s.Board.Progress(c.Job, worker, stageIdx)
				if _, _, all := s.Board.NextStage(c.Job, worker); all {
					s.Board.Done(c.Job, worker)
					s.Board.Accept(c.Job)
				}
			} else if !staged {
				s.Board.Done(c.Job, worker)
				// Only an accepted submission releases what waits on this job.
				if stored.Verified {
					s.Board.Accept(c.Job)
				}
			}
		}
	}

	// Verification already ran, so the worker can be told the outcome now,
	// standing where they took the photograph. Telling them later — or not at
	// all — is what makes a rejection unrecoverable: the one moment they can
	// fix a bad shot is before they walk away.
	out := map[string]any{
		"ok": true, "files": len(stored.Artifacts), "verified": stored.Verified,
	}
	if stored.Why != "" {
		out["why"] = stored.Why
	}
	if stored.ExpenseMinor > 0 {
		out["expense_minor"] = stored.ExpenseMinor
	}
	if staged {
		out["stage"] = stage.Name
		out["stages_total"] = len(l.Stages)
		if next, ns, all := s.Board.NextStage(c.Job, s.holderWorker(c)); all {
			out["remaining"] = 0
			out["status_note"] = "that was the last stage"
		} else {
			out["remaining"] = len(l.Stages) - next
			out["next_stage"] = ns.Name
			out["status_note"] = "next: " + ns.Name
		}
	}
	if stored.Reached != "" {
		out["reached"] = stored.Reached
	}
	if l, ok := s.Board.Get(c.Job); ok && l != nil {
		out["currency"] = l.Currency
		switch {
		case l.Practice:
			// A rehearsal. Nothing was held, nothing is paid, and nothing is
			// recorded against the worker's standing — said plainly, because
			// the alternative was a page reading "payment is still settling"
			// over an escrow of nothing.
			out["practice"] = true
			if stored.Verified {
				out["status"] = "practice run recorded"
			} else if stored.Why != "" {
				out["status"] = "not accepted"
			} else {
				out["status"] = "practice run recorded"
			}
			out["note"] = "This was a practice run. Nothing is paid for it and " +
				"it does not count toward your record either way."
		case !stored.Verified && stored.Why == "":
			// Nothing has looked at it yet. Saying "not accepted" here would
			// tell a worker they failed when in fact nobody has judged them,
			// which is the one message that would send them home for nothing.
			out["status"] = "submitted; payment follows verification"
		case !stored.Verified:
			out["status"] = "not accepted"
		case stored.Attempted:
			out["status"] = "attempt recorded"
			out["amount_minor"] = l.AttemptMinor
		default:
			out["status"] = "accepted"
			amount := l.PayMinor
			if l.Kind == KindObserve && stored.Finding {
				amount += l.BonusMinor
			}
			out["amount_minor"] = amount
		}
	} else if stored.Verified {
		out["status"] = "accepted"
	} else if stored.Why != "" {
		out["status"] = "not accepted"
	} else {
		out["status"] = "submitted; payment follows verification"
	}
	writeWork(w, http.StatusOK, out)
}

// sniff identifies the accepted formats by magic number and refuses anything
// else. An allowlist, because the failure mode of a denylist here is executing
// somebody's upload.
func sniff(b []byte) string {
	switch {
	case len(b) > 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "image/jpeg"
	case len(b) > 8 && string(b[1:4]) == "PNG":
		return "image/png"
	case len(b) > 12 && string(b[4:8]) == "ftyp":
		brand := string(b[8:12])
		switch {
		case strings.HasPrefix(brand, "heic"), strings.HasPrefix(brand, "heix"),
			strings.HasPrefix(brand, "mif1"), strings.HasPrefix(brand, "hevc"):
			return "image/heic"
		case strings.HasPrefix(brand, "isom"), strings.HasPrefix(brand, "mp4"),
			strings.HasPrefix(brand, "avc1"), strings.HasPrefix(brand, "qt  "):
			return "video/mp4"
		}
	}
	return ""
}

func (s *WorkServer) handlePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	fmt.Fprint(w, workPageHTML)
}

func writeWork(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// holderWorker resolves a capability back to the worker it was issued to.
//
// Stage progress is recorded against the person holding the seat, and the
// upload path knows only the capability.
func (s *WorkServer) holderWorker(c *Capability) string {
	if s.Board == nil {
		return c.Holder
	}
	if w, ok := s.Board.WorkerFor(c.Holder); ok {
		return w
	}
	return c.Holder
}

// ImageSignals is what the blind describer thought about the imagery itself,
// as opposed to what it showed.
type ImageSignals struct {
	// Synthetic is the highest suspicion across the files that this is a
	// generated image rather than a photograph. Enforced.
	Synthetic float64 `json:"synthetic"`
	// Recapture is the highest suspicion that a file is a photograph of a
	// screen or of a print. Recorded, not enforced — nobody has measured what
	// separates it from an honest photograph taken at an angle.
	Recapture float64 `json:"recapture"`
	// InstructionLike marks text in the frame that reads as aimed at the
	// adjudicator rather than describing the scene.
	InstructionLike bool `json:"instruction_like,omitempty"`
}
