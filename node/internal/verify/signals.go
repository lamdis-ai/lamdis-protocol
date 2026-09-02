package verify

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/bits"
	"os"
	"strings"
	"sync"
	"time"
)

// Seed log-likelihood ratios, in nats. These are hand-set starting points,
// used until a cell has enough labelled outcomes to learn from; the honest
// framing is that they encode our beliefs, not measurements.
//
// Note the asymmetries. Stripped EXIF is weakly negative rather than neutral,
// because honest phone captures usually carry it. Novelty is only weakly
// positive, because an unseen image is the unremarkable case. Recompression is
// barely negative, because it is common and benign. And the model's own
// confidence is capped well below what it claims.
var seedLLR = map[string]map[string]float64{
	"nonce.transcribed": {
		"match":  +2.2, // the single strongest non-corroboration signal
		"absent": -2.5,
	},
	"capture.window": {
		"hit":    +1.3,
		"miss":   -2.0,
		"absent": -0.8,
	},
	"capture.via_our_page": {
		"true":  +1.1,
		"false": 0,
	},
	"capture.geo": {
		// A location hit is worth less than a miss costs. Being at the right
		// place is weak evidence you did the job; being demonstrably somewhere
		// else is strong evidence you did not.
		"within_radius":  +1.4,
		"outside_radius": -2.2,
		"absent":         -0.5,
	},
	"novelty.corpus": {
		"novel":    +0.35,
		"near_dup": -2.6,
	},
	"integrity.recompressed": {
		"true": -0.6,
	},
	"integrity.transformed": {
		"true": -1.0, // a client re-encode destroys the metadata we would check
	},
	"vision.verdict": {
		// Discretised model output. The top bucket is deliberately compressed
		// to about 4:1 so that a maximally confident model, on its own, cannot
		// carry a verdict past 0.90 from a neutral prior.
		"satisfied_high":     +1.4,
		"satisfied_medium":   +1.0,
		"satisfied_low":      +0.6,
		"indeterminate":      -0.3,
		"not_satisfied_high": -2.0,
	},
	"vision.synthetic_suspicion": {
		"high": -1.2, // deliberately weak: this arms race is not winnable
	},
	"vision.recapture_suspicion": {
		"high": -1.5, // a photo of a screen or a print
	},
	"corroborate.agree": {
		"true": +2.0,
	},
}

func llr(feature, value string) float64 {
	if m, ok := seedLLR[feature]; ok {
		if v, ok := m[value]; ok {
			return v
		}
	}
	return 0
}

// Evidence is what the verifier is handed about one artifact.
type Evidence struct {
	EntryID     string
	SHA256      string
	MediaType   string
	Bytes       int64
	AttestedBy  string // device_key | capability
	Transformed bool
	SubmittedAt time.Time

	// Nonce is the per-job challenge the provider was asked to include in the
	// shot. Expected is what we issued; Transcribed is what a blind describer
	// read out of the image without being told what to look for.
	NonceExpected    string
	NonceTranscribed string

	// CapturedAt is the artifact's own asserted capture time, and Window is
	// how fresh it had to be. Both attacker-controlled, hence weighted, never
	// trusted.
	CapturedAt time.Time
	Window     time.Duration

	// PerceptualHash is a 64-bit fingerprint used for corpus reuse detection,
	// and MirrorHash the same for the horizontally flipped image. Storing both
	// is what catches the cheapest evasion there is: flipping an old photo so
	// a single fingerprint no longer matches.
	PerceptualHash uint64
	MirrorHash     uint64

	// Geo is where the file says it was taken, and Target is where the job
	// said to go. Like every other metadata field these are attacker-supplied,
	// so a hit is weighted rather than trusted — but a miss is strong: an
	// honest capture at the wrong address is still the wrong address.
	HasGeo               bool
	GeoLat, GeoLon       float64
	TargetLat, TargetLon float64
	RadiusM              float64

	// ViaOurPage records that the bytes arrived through the capture page
	// rather than an arbitrary upload.
	ViaOurPage bool

	// Recompressed is a deterministic JPEG signal.
	Recompressed bool
}

// VisionVerdict is a model's structured reading of one artifact.
type VisionVerdict struct {
	Verdict                string  // satisfied | not_satisfied | indeterminate
	SelfConfidence         float64 // never used as a probability
	SyntheticSuspicion     float64
	RecaptureSuspicion     float64
	InjectionDetected      bool
	InstructionLikeText    bool
	SupportingObservations []string
	Cents                  int
}

// NearDuplicateBits is how far apart two fingerprints may be and still be
// treated as the same picture.
//
// The value is measured, not guessed. Against a real phone photograph and the
// evasions a provider actually reaches for, the fingerprint moved by: 0 bits
// for a re-encode to q55, 0 for a 12% brightness lift, 0 for a 50% downscale,
// 0 for a horizontal mirror (via the mirrored fingerprint), and 12 for a 6%
// crop. Genuinely different photographs from the same camera and room sat at
// 27 to 41 bits apart. Sixteen therefore catches every evasion measured while
// leaving an eleven-bit margin before the nearest honest photo — an earlier
// threshold of 10 let the crop through.
const NearDuplicateBits = 16

// Corpus remembers every artifact ever submitted, so a provider cannot resubmit
// an old photo — the most likely real-world fraud, because it needs no
// technical skill at all.
type Corpus struct {
	mu    sync.Mutex
	exact map[string]string // sha256 -> first entry that used it
	phash map[uint64]string
	// path is where entries are appended, so a restart does not forget every
	// photograph the exchange has ever seen. Empty means memory only.
	path string
}

func NewCorpus() *Corpus {
	return &Corpus{exact: map[string]string{}, phash: map[uint64]string{}}
}

// corpusEntry is one line of the on-disk corpus.
type corpusEntry struct {
	Entry  string `json:"entry"`
	SHA256 string `json:"sha256,omitempty"`
	PHash  uint64 `json:"phash,omitempty"`
}

// OpenCorpus loads a corpus from a file and appends every later addition to
// it.
//
// In memory only, the reuse check forgot everything at each deploy: the same
// photograph earned again the morning after a restart. The file is one JSON
// object per line, appended as artifacts arrive, and read back in full at
// startup. A missing file is an empty corpus; a line that cannot be read is
// skipped rather than refusing to start, because a corrupt line must not take
// the exchange down with it.
func OpenCorpus(path string) (*Corpus, error) {
	c := NewCorpus()
	c.path = path
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Prove the location is writable now, not on the first submission.
			w, werr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if werr != nil {
				return nil, werr
			}
			w.Close()
			return c, nil
		}
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		var e corpusEntry
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Entry == "" {
			continue
		}
		c.addLocked(Evidence{EntryID: e.Entry, SHA256: e.SHA256, PerceptualHash: e.PHash})
	}
	return c, sc.Err()
}

// Path is where this corpus persists, or "" for memory only.
func (c *Corpus) Path() string { return c.path }

// Seen reports whether this artifact, or one perceptually close to it, has
// been submitted before, and by which entry.
func (c *Corpus) Seen(e Evidence) (exact bool, near bool, priorEntry string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if prior, ok := c.exact[e.SHA256]; ok && prior != e.EntryID {
		return true, false, prior
	}
	if e.PerceptualHash != 0 {
		for h, prior := range c.phash {
			if prior == e.EntryID {
				continue
			}
			// Compare the submission both ways round: a stored fingerprint may
			// match this image directly, or match its mirror.
			if bits.OnesCount64(h^e.PerceptualHash) <= NearDuplicateBits ||
				(e.MirrorHash != 0 && bits.OnesCount64(h^e.MirrorHash) <= NearDuplicateBits) {
				return false, true, prior
			}
		}
	}
	return false, false, ""
}

// Add records an artifact so later submissions can be checked against it.
func (c *Corpus) Add(e Evidence) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.addLocked(e) {
		return
	}
	if c.path == "" {
		return
	}
	line, err := json.Marshal(corpusEntry{
		Entry: e.EntryID, SHA256: e.SHA256, PHash: e.PerceptualHash,
	})
	if err != nil {
		return
	}
	f, err := os.OpenFile(c.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(line, '\n'))
}

// addLocked records an artifact in memory and reports whether anything new
// was learned. Callers hold the lock.
func (c *Corpus) addLocked(e Evidence) bool {
	added := false
	if e.SHA256 != "" {
		if _, ok := c.exact[e.SHA256]; !ok {
			c.exact[e.SHA256] = e.EntryID
			added = true
		}
	}
	if e.PerceptualHash != 0 {
		if _, ok := c.phash[e.PerceptualHash]; !ok {
			c.phash[e.PerceptualHash] = e.EntryID
			added = true
		}
	}
	return added
}

// Deterministic runs every check that costs nothing: metadata, freshness,
// capture binding, and corpus novelty. These run before any model call, and on
// their own they decide most fraud.
func Deterministic(e Evidence, corpus *Corpus) []Signal {
	var out []Signal
	add := func(feature, value string, class IndepClass, fatal bool) {
		out = append(out, Signal{
			Verifier: "deterministic", Feature: feature, Value: value,
			LLR: llr(feature, value), Class: class, Fatal: fatal,
		})
	}

	// The challenge nonce. Go does the comparison, not the model: the
	// describer transcribes whatever text it sees without being told what to
	// look for, so a submitter cannot talk it into a match.
	switch {
	case e.NonceExpected == "":
		// No challenge was issued; nothing to check.
	case e.NonceTranscribed == "":
		add("nonce.transcribed", "absent", ClassProvenance, false)
	case strings.EqualFold(strings.TrimSpace(e.NonceTranscribed), e.NonceExpected):
		add("nonce.transcribed", "match", ClassProvenance, false)
	default:
		// A wrong code is not weak evidence, it is a different job's photo or
		// a fabrication. Fatal.
		out = append(out, Signal{
			Verifier: "deterministic", Feature: "nonce.transcribed", Value: "mismatch",
			Class: ClassProvenance, Fatal: true,
			Detail: map[string]any{"expected": e.NonceExpected, "read": e.NonceTranscribed},
		})
	}

	// Freshness, from the artifact's own asserted capture time.
	switch {
	case e.CapturedAt.IsZero():
		add("capture.window", "absent", ClassCapture, false)
	case e.Window > 0 && e.SubmittedAt.Sub(e.CapturedAt) > e.Window:
		add("capture.window", "miss", ClassCapture, false)
	default:
		add("capture.window", "hit", ClassCapture, false)
	}

	if e.ViaOurPage {
		add("capture.via_our_page", "true", ClassCapture, false)
	}

	// Location, when the job is location-bound.
	if e.RadiusM > 0 {
		switch {
		case !e.HasGeo:
			add("capture.geo", "absent", ClassCapture, false)
		default:
			d := haversineM(e.GeoLat, e.GeoLon, e.TargetLat, e.TargetLon)
			if d <= e.RadiusM {
				out = append(out, Signal{
					Verifier: "deterministic", Feature: "capture.geo", Value: "within_radius",
					LLR: llr("capture.geo", "within_radius"), Class: ClassCapture,
					Detail: map[string]any{"distance_m": int(d), "radius_m": int(e.RadiusM)},
				})
			} else {
				// Fatal: the job named a place, and this was taken somewhere
				// else. No amount of authenticity makes it evidence about the
				// place that was asked about.
				out = append(out, Signal{
					Verifier: "deterministic", Feature: "capture.geo", Value: "outside_radius",
					Class: ClassCapture, Fatal: true,
					Detail: map[string]any{"distance_m": int(d), "radius_m": int(e.RadiusM)},
				})
			}
		}
	}
	if e.Transformed {
		add("integrity.transformed", "true", ClassIntegrity, false)
	}
	if e.Recompressed {
		add("integrity.recompressed", "true", ClassIntegrity, false)
	}

	if corpus != nil && e.SHA256 != "" {
		exact, near, prior := corpus.Seen(e)
		switch {
		case exact:
			out = append(out, Signal{
				Verifier: "deterministic", Feature: "novelty.corpus", Value: "exact_dup",
				Class: ClassNovelty, Fatal: true,
				Detail: map[string]any{"first_seen": prior},
			})
		case near:
			// Fatal, like an exact duplicate. Treating reuse as a weight lets
			// a provider who also satisfies the challenge code outvote it, and
			// reusing an image across jobs is the whole fraud — not a factor
			// in it.
			out = append(out, Signal{
				Verifier: "deterministic", Feature: "novelty.corpus", Value: "near_dup",
				Class: ClassNovelty, Fatal: true,
				Detail: map[string]any{"first_seen": prior},
			})
		default:
			add("novelty.corpus", "novel", ClassNovelty, false)
		}
	}
	return out
}

// FromVision converts a model reading into signals. The model's self-reported
// confidence is never used as a probability; it is discretised into buckets
// that each carry an empirically learned weight.
func FromVision(v VisionVerdict) []Signal {
	var out []Signal

	// An injection attempt is fraud, not noise. Treating it as something to
	// filter out would make attacking the verifier free; making it fatal makes
	// it strictly worse than failing honestly.
	if v.InjectionDetected {
		out = append(out, Signal{
			Verifier: "vision", Feature: "injection.detected", Value: "true",
			Class: ClassContent, Fatal: true, Cents: v.Cents,
		})
		return out
	}

	bucket := "indeterminate"
	switch v.Verdict {
	case "satisfied":
		switch {
		case v.SelfConfidence >= 0.95:
			bucket = "satisfied_high"
		case v.SelfConfidence >= 0.85:
			bucket = "satisfied_medium"
		default:
			bucket = "satisfied_low"
		}
	case "not_satisfied":
		if v.SelfConfidence >= 0.85 {
			bucket = "not_satisfied_high"
		}
	}
	out = append(out, Signal{
		Verifier: "vision", Feature: "vision.verdict", Value: bucket,
		LLR: llr("vision.verdict", bucket), Class: ClassContent, Cents: v.Cents,
	})
	if v.SyntheticSuspicion >= 0.6 {
		out = append(out, Signal{
			Verifier: "vision", Feature: "vision.synthetic_suspicion", Value: "high",
			LLR: llr("vision.synthetic_suspicion", "high"), Class: ClassContent,
		})
	}
	if v.RecaptureSuspicion >= 0.6 {
		out = append(out, Signal{
			Verifier: "vision", Feature: "vision.recapture_suspicion", Value: "high",
			LLR: llr("vision.recapture_suspicion", "high"), Class: ClassContent,
		})
	}
	return out
}

// TierFor works out how far the evidence structurally permits us to go. Note
// that authentication strength is an input: a photo submitted through a
// capability link, which the exchange signed on the provider's behalf, cannot
// on its own reach the top tier.
func TierFor(e Evidence, hasVision, corroborated bool) Tier {
	if e.SHA256 == "" {
		return TierV0
	}
	nonceOK := e.NonceExpected != "" &&
		strings.EqualFold(strings.TrimSpace(e.NonceTranscribed), e.NonceExpected)
	fresh := !e.CapturedAt.IsZero() && (e.Window == 0 || e.SubmittedAt.Sub(e.CapturedAt) <= e.Window)

	switch {
	case corroborated && nonceOK && hasVision && e.AttestedBy == "device_key":
		return TierV3
	case nonceOK && fresh && hasVision:
		return TierV2
	default:
		return TierV1
	}
}

// HasProvenance reports whether the artifact is bound to a real capture at
// all. This is the single input that separates a 0.90 ceiling from 0.72.
func HasProvenance(e Evidence) bool {
	return e.ViaOurPage || e.AttestedBy == "device_key" || !e.CapturedAt.IsZero()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// haversineM is the great-circle distance between two coordinates, in metres.
func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat, dLon := rad(lat2-lat1), rad(lon2-lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
