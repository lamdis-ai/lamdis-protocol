package exchange

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/media"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/evidence"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/verify"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/vision"
)

// SubmissionVerifier decides whether an uploaded artifact is evidence.
//
// The check that matters is the challenge code. A photograph of the right
// building is compatible with having gone there today, having gone last year,
// or having found the picture online; a photograph containing a six-character
// code that was generated privately for this claimant, minutes ago, is not.
// Everything else here is cheaper and weaker.
//
// The describer is never told what code to look for. It transcribes whatever
// text it can see, and Go does the comparison — so a submitter cannot talk the
// model into agreeing that their photo contains a code it does not.
type SubmissionVerifier struct {
	// Vision reads the image. Without one, nothing can confirm a challenge and
	// no submission becomes payable.
	Vision vision.Model
	// Corpus catches an artifact that has been submitted before, which on an
	// open board is the obvious way to earn without going anywhere.
	Corpus *verify.Corpus
	// Media decomposes video into frames and audio. Nil means video cannot be
	// accepted, and the worker is told so rather than silently failing.
	Media media.Extractor
	// Transcriber reads a soundtrack. Nil means a spoken code cannot count.
	Transcriber media.Transcriber
	// StoreFrame persists a still pulled from a video, so a human reviewer can
	// see what the model saw.
	StoreFrame func(job, sha string, jpeg []byte) error
	// Predicate returns what a job asked to become true, so the evidence can
	// be judged against it rather than merely tied to it.
	Predicate func(job string) (predicate, kind string, ok bool)
	// StageDeliverable returns what one stage of a longer job was supposed to
	// show, when the job is cut into stages.
	StageDeliverable func(job string, stage int) (string, bool)

	mu sync.Mutex
	// seen is what the blind describer read out of this submission's files.
	seen []*vision.Observation
}

// Verify is the hook Server.Verify expects.
//
// A submission passes when the challenge code we privately issued this worker
// is found in at least one artifact — read out of a picture by a describer
// that was never told what to look for, or heard in a video's audio. Finding
// it nowhere is not a judgement about the predicate; it is the observation
// that this evidence is not tied to this job, and evidence untied to a job
// could have been taken anywhere, any time.
func (sv *SubmissionVerifier) Verify(sub api.Submission, blob func(string) ([]byte, bool)) (api.Submission, error) {
	if len(sub.Artifacts) == 0 {
		if len(sub.Report) > 0 {
			return sv.verifyReport(sub)
		}
		return sub, fmt.Errorf("nothing was uploaded")
	}

	found := false
	for i := range sub.Artifacts {
		a := &sub.Artifacts[i]
		data, ok := blob(a.SHA256)
		if !ok {
			return sub, fmt.Errorf("one of those files went missing before it could be checked")
		}
		switch a.Kind {
		case media.KindVideo:
			if err := sv.readVideo(a, data, sub); err != nil {
				return sub, err
			}
		default:
			if err := sv.readImage(a, data, sub); err != nil {
				return sub, err
			}
		}
		if a.ChallengeSeen != "" {
			found = true
		}
	}

	// Not finding the code and not being able to look for it are different
	// answers. The first is a refusal the worker can act on by retaking the
	// shot; the second is our problem, and rejecting their work for it would
	// be blaming them for our missing configuration. Either way nothing is
	// payable, because nothing has been established.
	if !sv.canCheck() {
		sub.Why = "no verifier is configured on this exchange, so nothing has been checked"
		return sub, nil
	}
	// Where a job named a place, evidence from somewhere else is not evidence
	// of it. A miss here is strong even though a hit is only weak: metadata is
	// attacker-supplied, so being at the right place proves little, but being
	// demonstrably two towns away proves a great deal.
	if sub.RadiusM > 0 {
		if err := checkGeofence(&sub); err != nil {
			sub.Why = err.Error()
			return sub, err
		}
	}
	// A tier is a promise about what the evidence can support, and it has to
	// bite.
	//
	// The geofence returns cleanly when no file carries a location — phones
	// and messaging apps really do strip it — and a comment claimed this
	// "caps what the evidence can prove rather than rejecting it". Nothing
	// capped anything: Verified was set either way, so stripping metadata
	// turned a V2 job into an unfenced one and any paved driveway on earth
	// would do.
	if err := checkProvenance(&sub); err != nil {
		sub.Why = err.Error()
		return sub, err
	}
	if !found {
		sub.Why = fmt.Sprintf(
			"the code %s was not legible in any photo, and not audible in any video",
			sub.Challenge)
		return sub, fmt.Errorf("%s", sub.Why)
	}
	// Whose lawn is it.
	//
	// Everything above establishes that a photograph was taken recently, by
	// somebody holding a code, of something that matches the predicate. None
	// of it establishes *where*. A cut lawn is a cut lawn; the geofence reads
	// EXIF, which whoever made the file chose what to write.
	//
	// So a mark that belongs to the property has to be legible. The failure is
	// soft on a derived mark and hard on one the buyer stated, because those
	// are different claims: the buyer knows whether their number can be seen
	// from the work, and the exchange inferring one from an address is
	// guessing. Refusing honest work over our own guess would be the worse
	// error, so a derived mark that is missing is recorded and capped rather
	// than rejected.
	if m := sub.SiteMark; m != nil {
		sub.MarkSeen = api.MarkSeenIn(sv.allTranscribedText(), m.Text)
		// An observation pays on admissibility alone — there is no adjudicated
		// finding standing between "the code was legible" and the fee, as
		// there is for a do-job. So for an observation of a place, the mark
		// is the only thing saying the photograph is of that place, and a
		// derived one is required too. A code card photographed at home with
		// edited coordinates was earning every observe fee on the board.
		kind, _ := sv.kindOf(sub.Job)
		if !sub.MarkSeen && (!m.Derived || kind == api.KindObserve) {
			sub.Why = fmt.Sprintf(
				"none of the photographs show %s, so there is nothing tying them "+
					"to this property rather than a similar one. Include it in one "+
					"shot — %s", m.Text, m.Note)
			return sub, fmt.Errorf("%s", sub.Why)
		}
	}

	// Fabricated imagery.
	//
	// The describer scores this on every image and, until it was measured,
	// nothing consumed it — so a generated photograph faced only the challenge
	// code and the geofence, both of which a fabrication satisfies trivially.
	//
	// The threshold is measured, not guessed. See SyntheticThreshold.
	sub.Signals = sv.signals()
	if score, ok := sv.mostSynthetic(); ok && score >= SyntheticThreshold {
		sub.Why = "these photographs look generated rather than taken. If that " +
			"is wrong, send the originals straight from your camera roll " +
			"without editing or re-saving them"
		return sub, fmt.Errorf("%s", sub.Why)
	}

	// Admissible. That is not the same as done.
	//
	// Until now this was the last step: a do-job paid its full completion fee
	// the moment somebody proved they had been to the address with the code in
	// frame. Nothing looked at whether the bins had actually been moved. The
	// exchange sells verified outcomes and was paying for verified presence.
	//
	// Adjudicate closes that gap. The describer never saw the predicate, so
	// its account is independent of the answer; the judge sees the predicate
	// and the description, and never the raw image.
	sub.Verified = true

	// An attempt is a claim about the world too, and it has to be evidenced.
	//
	// Nothing evaluated it: the worker's stated reason was stored, shown to
	// the buyer, and never judged, so "the gate was padlocked" and "nope" paid
	// identically. Presence plus an arbitrary sentence collected the attempt
	// fee.
	//
	// The predicate here is fixed rather than built from the worker's words.
	// Their text is attacker-controlled and the predicate is the adjudicator's
	// trusted slot; feeding one into the other would hand a submitter the
	// instruction channel. What they wrote still reaches the buyer — it is
	// simply not what the model is asked about.
	if sub.Attempted {
		sub.Finding = sv.judgeAgainst(&sub, attemptPredicate)
		if !sub.Finding {
			sub.Why = "the photographs do not show anything that would have " +
				"stopped the work. Photograph the obstruction itself — the locked " +
				"gate, the closed shutter, the missing unit — with the code in frame"
			return sub, fmt.Errorf("%s", sub.Why)
		}
		return sub, nil
	}

	sub.Finding = sv.judge(&sub)

	// Say which half failed. "Not accepted" covers two very different
	// situations — the code was unreadable, or the work is not visibly done —
	// and only one of them is fixed by taking a better photograph.
	if kind, ok := sv.kindOf(sub.Job); ok && kind == api.KindDo && !sub.Finding {
		what := "the job finished"
		if sub.StageName != "" {
			what = strings.ToLower(sub.StageName) + " finished"
		}
		sub.Why = "the photographs place you there, but they do not show " + what +
			". Reshoot so the finished work is clearly in frame, or mark it as an " +
			"attempt if it could not be done."
	}
	return sub, nil
}

// verifyReport accepts a written answer with no photograph behind it.
//
// Honestly: nothing here is verified in the sense the rest of this file means.
// No challenge code ties it to a time, no metadata to a place, no describer
// read anything. What is established is that the person holding this job's
// capability wrote these answers, which is a signed claim — tier V0 — and the
// submission says so rather than inheriting the tier the buyer asked for.
// It is admissible because that is exactly what the buyer bought when they
// asked for a table instead of a photograph.
func (sv *SubmissionVerifier) verifyReport(sub api.Submission) (api.Submission, error) {
	sub.Verified = true
	sub.Reached = string(verify.TierV0)
	sub.Finding = false
	return sub, nil
}

// judge decides whether the evidence shows what the job asked for.
//
// A do-job that is admissible but does not show the work is not a completion,
// and paying it as one is how a marketplace stops meaning anything. An
// observation records the finding and pays its bonus on it.
// kindOf reports a job's kind without assuming the hook is wired.
//
// A nil Predicate is a development setup, not a reason to crash on every
// submission the exchange receives.
func (sv *SubmissionVerifier) kindOf(job string) (string, bool) {
	if sv.Predicate == nil {
		return "", false
	}
	_, kind, ok := sv.Predicate(job)
	return kind, ok
}

// attemptPredicate is what a wasted trip has to look like.
//
// Fixed wording, deliberately: see the note where it is used.
const attemptPredicate = "the photograph shows a physical obstruction, closure, " +
	"absence or condition at the location that would prevent the described work " +
	"from being carried out"

func (sv *SubmissionVerifier) judge(sub *api.Submission) bool {
	// A nil hook is a development setup, not a reason to crash on every
	// submission the exchange receives. The guard used to live in
	// judgeAgainst and did not come with the lookup when it moved.
	if sv.Predicate == nil {
		return false
	}
	predicate, _, ok := sv.Predicate(sub.Job)
	if !ok || predicate == "" {
		return false
	}
	// On a staged job the question is whether *this stage* is done. Judging
	// the base course against "the driveway is paved" would refuse honest
	// work, and judging the final stage too loosely would accept a photograph
	// of a driveway that was already fine.
	if sv.StageDeliverable != nil {
		if d, ok := sv.StageDeliverable(sub.Job, sub.Stage); ok && d != "" {
			predicate = d
		}
	}
	return sv.judgeAgainst(sub, predicate)
}

// judgeAgainst asks the adjudicator about one specific claim.
func (sv *SubmissionVerifier) judgeAgainst(sub *api.Submission, predicate string) bool {
	obs := sv.takeSeen()
	if sv.Vision == nil || predicate == "" || len(obs) == 0 {
		// Nothing to judge with. Left as not-found rather than assumed true:
		// an unjudged do-job must not pay as a completed one, and an unjudged
		// attempt must not pay as a wasted trip.
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for _, o := range obs {
		adj, _, err := sv.Vision.Adjudicate(ctx, predicate, o)
		if err != nil || adj == nil {
			continue
		}
		// Any one file showing it is enough: a submission is several angles of
		// one moment, and the buyer asked for the thing to be true, not for
		// every photograph to show it.
		// An injection attempt is a reason to distrust the whole file, not a
		// verdict to act on: the describer flagged text aimed at the judge.
		if adj.InjectionAttemptDetected {
			continue
		}
		if adj.Verdict == "satisfied" {
			return true
		}
	}
	return false
}

// checkGeofence refuses evidence whose own metadata places it outside the
// area the job named.
func checkGeofence(sub *api.Submission) error {
	var located, inside int
	for _, a := range sub.Artifacts {
		if !a.HasGeo {
			continue
		}
		located++
		if haversineM(float64(a.LatE7)/1e7, float64(a.LonE7)/1e7,
			float64(sub.LatE7)/1e7, float64(sub.LonE7)/1e7) <= float64(sub.RadiusM) {
			inside++
		}
	}
	if located == 0 {
		// Nothing carried a location. That is common and not an accusation —
		// phones strip it, apps re-encode it away — so it caps what the
		// evidence can prove rather than rejecting it.
		return nil
	}
	if inside == 0 {
		return fmt.Errorf(
			"every photo that recorded a location was taken outside the area for this job")
	}
	return nil
}

// haversineM is the great-circle distance in metres.
func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat, dLon := rad(lat2-lat1), rad(lon2-lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// canCheck reports whether anything here is able to look for a challenge code.
func (sv *SubmissionVerifier) canCheck() bool {
	return sv.Vision != nil || sv.Transcriber != nil
}

// readImage checks one still.
func (sv *SubmissionVerifier) readImage(a *api.Artifact, data []byte, sub api.Submission) error {
	art, err := evidence.Analyze(data, a.Mime)
	if err != nil {
		return fmt.Errorf("one of those files could not be read as an image")
	}
	if err := sv.checkReuse(art, a, sub); err != nil {
		return err
	}
	if ex, err := evidence.ParseEXIF(data); err == nil {
		a.CapturedAt = ex.DateTimeOriginal
		if ex.HasGPS {
			a.LatE7 = int64(ex.Lat * 1e7)
			a.LonE7 = int64(ex.Lon * 1e7)
			a.HasGeo = true
		}
	}
	if sv.Vision == nil {
		return nil // nothing looked at it; the submission stays unverified
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	obs, _, err := sv.Vision.Describe(ctx, data)
	if err != nil {
		return fmt.Errorf("that image could not be checked right now")
	}
	if containsChallenge(obs, sub.Challenge) {
		a.ChallengeSeen = "text"
		sv.remember(art, a, sub)
	}
	sv.keep(obs)
	return nil
}

// keep collects what the blind describer saw, for the adjudication step.
//
// The describer is not told the predicate, which is the whole reason its
// account can be trusted: it transcribes what is in front of it without
// knowing what answer would be convenient.
func (sv *SubmissionVerifier) keep(obs *vision.Observation) {
	if obs == nil {
		return
	}
	sv.mu.Lock()
	sv.seen = append(sv.seen, obs)
	sv.mu.Unlock()
}

// SyntheticThreshold is the synthetic_suspicion score at which evidence is
// refused as fabricated.
//
// Measured on 2026-08-21 against the live describer rather than chosen:
//
//	18 genuine phone photographs   max 0.30, median 0.05
//	Qwen diffusion, plain scenes   0.45, 0.60
//	Qwen diffusion, odd framing    0.75, 0.75
//	flat vector illustrations      0.97-0.98
//
// Real tops out at 0.30 and fabricated starts at 0.45, so anything in 0.31
// to 0.44 separates the two sets. 0.40 sits near the top of that band on
// purpose: refusing an honest worker is the worse error, and the sample is
// small — 18 real images, one generator, no adversarial tuning.
//
// The constant that was here before this was measured was 0.60, which would
// have passed the 0.45 image.
const SyntheticThreshold = 0.40

// RecaptureThreshold would refuse a photograph of a screen or of a print,
// which is the cheapest way to deliver a fabricated image without generating
// anything.
//
// It is deliberately not enforced: nobody has measured it, and wiring a
// plausible-looking number is exactly the mistake SyntheticThreshold was
// rescued from. The signal is recorded on every submission so the corpus
// exists when somebody does measure it.
const RecaptureThreshold = 0.0

// signals records what the describer thought of the imagery, enforced or not.
//
// Kept on every submission because the unmeasured thresholds cannot be
// measured without a corpus, and there is no corpus unless the numbers are
// stored while they are still useless.
func (sv *SubmissionVerifier) signals() *api.ImageSignals {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if len(sv.seen) == 0 {
		return nil
	}
	out := &api.ImageSignals{}
	for _, o := range sv.seen {
		if o == nil {
			continue
		}
		if o.SyntheticSuspicion > out.Synthetic {
			out.Synthetic = o.SyntheticSuspicion
		}
		if o.RecaptureSuspicion > out.Recapture {
			out.Recapture = o.RecaptureSuspicion
		}
		if o.InstructionLikeText {
			out.InstructionLike = true
		}
	}
	return out
}

// mostSynthetic is the highest suspicion across the files in this submission.
//
// The maximum rather than the mean: a submission is one claim, and a single
// fabricated frame among honest ones is still a fabricated claim.
func (sv *SubmissionVerifier) mostSynthetic() (float64, bool) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	worst, found := 0.0, false
	for _, o := range sv.seen {
		if o == nil {
			continue
		}
		found = true
		if o.SyntheticSuspicion > worst {
			worst = o.SyntheticSuspicion
		}
	}
	return worst, found
}

func (sv *SubmissionVerifier) takeSeen() []*vision.Observation {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	out := sv.seen
	sv.seen = nil
	return out
}

// readVideo decomposes a clip and checks what comes out of it.
//
// Both halves are tried because they fail in different ways: a code written on
// paper can be out of focus, and a code spoken aloud can be drowned by traffic.
// Either one establishes the tie, and a clip carrying both is stronger than a
// photograph can be.
func (sv *SubmissionVerifier) readVideo(a *api.Artifact, data []byte, sub api.Submission) error {
	if sv.Media == nil {
		return fmt.Errorf("this exchange cannot check video yet; please upload photos")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	probe, err := sv.Media.Probe(ctx, data, a.Mime)
	if err != nil {
		return fmt.Errorf("that video could not be read")
	}
	a.DurationMS = probe.Duration.Milliseconds()

	frames, err := sv.Media.Frames(ctx, data, a.Mime, media.MaxFrames)
	if err == nil {
		for _, fr := range frames {
			sum := sha256.Sum256(fr.JPEG)
			sha := hex.EncodeToString(sum[:])
			a.Frames = append(a.Frames, sha)
			if sv.StoreFrame != nil {
				_ = sv.StoreFrame(sub.Job, sha, fr.JPEG)
			}
			if sv.Vision == nil || a.ChallengeSeen != "" {
				continue
			}
			obs, _, err := sv.Vision.Describe(ctx, fr.JPEG)
			if err == nil && containsChallenge(obs, sub.Challenge) {
				a.ChallengeSeen = "text"
			}
		}
	}

	// The soundtrack, if there is one and we can read it.
	if probe.HasAudio && sv.Transcriber != nil {
		if wav, err := sv.Media.Audio(ctx, data, a.Mime); err == nil {
			if tr, err := sv.Transcriber.Transcribe(ctx, wav); err == nil {
				a.Transcript = tr.Text
				if a.ChallengeSeen == "" &&
					strings.Contains(normalizeCode(tr.Text), normalizeCode(sub.Challenge)) {
					a.ChallengeSeen = "spoken"
				}
			}
		}
	}
	return nil
}

// checkReuse refuses an artifact that has been submitted before. On an open
// board this is the cheapest fraud to attempt and the cheapest to detect.
func (sv *SubmissionVerifier) checkReuse(art evidence.Artifact, a *api.Artifact, sub api.Submission) error {
	if sv.Corpus == nil {
		return nil
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	exact, near, prior := sv.Corpus.Seen(verify.Evidence{
		EntryID: submissionID(sub), SHA256: art.SHA256, MediaType: a.Mime,
		Bytes: int64(a.Bytes), AttestedBy: sub.AttestedBy, SubmittedAt: sub.At,
		PerceptualHash: art.DHash, MirrorHash: art.MirrorHash,
	})
	if exact || near {
		return fmt.Errorf("one of those images has been submitted before (%s)", prior)
	}
	return nil
}

func (sv *SubmissionVerifier) remember(art evidence.Artifact, a *api.Artifact, sub api.Submission) {
	if sv.Corpus == nil {
		return
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	sv.Corpus.Add(verify.Evidence{
		EntryID: submissionID(sub), SHA256: art.SHA256, MediaType: a.Mime,
		Bytes: int64(a.Bytes), AttestedBy: sub.AttestedBy, SubmittedAt: sub.At,
		PerceptualHash: art.DHash, MirrorHash: art.MirrorHash,
		NonceExpected: sub.Challenge, NonceTranscribed: sub.Challenge,
	})
}

// submissionID names one submission. The capability holder is what
// distinguishes two workers submitting the same photograph, which is the case
// reuse detection exists to catch.
func submissionID(sub api.Submission) string {
	// The stage belongs in this key.
	//
	// Without it every stage of a job shared one identity, and the reuse
	// corpus deliberately skips matches whose EntryID is the same as the
	// incoming one — so submitting a byte-identical photograph for all four
	// stages of a driveway was not merely undetected, it was ignored on
	// purpose. Exact hash, perceptual hash and mirror hash all passed it
	// through.
	return fmt.Sprintf("%s:%s#%d", sub.Job, sub.Holder, sub.Stage)
}

// containsChallenge looks for the issued code among the text the describer
// read out of the image.
func containsChallenge(obs *vision.Observation, challenge string) bool {
	if obs == nil || challenge == "" {
		return false
	}
	want := normalizeCode(challenge)
	if want == "" {
		return false
	}
	for _, t := range obs.TextVisible {
		if strings.Contains(normalizeCode(t.Text), want) {
			return true
		}
	}
	for _, s := range obs.Signage {
		if strings.Contains(normalizeCode(s.Text), want) {
			return true
		}
	}
	return false
}

// allTranscribedText is every string the describers read out of the evidence.
//
// Collected from the blind pass, which never knew what was being looked for.
// That is the whole reason the comparison can be trusted: a submitter cannot
// persuade a describer to report a house number it did not see, because the
// describer was not told which number pays.
func (sv *SubmissionVerifier) allTranscribedText() []string {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var out []string
	for _, obs := range sv.seen {
		if obs == nil {
			continue
		}
		for _, t := range obs.TextVisible {
			out = append(out, t.Text)
		}
		for _, g := range obs.Signage {
			out = append(out, g.Text)
		}
	}
	return out
}

// normalizeCode folds the confusions an OCR pass makes on handwriting.
//
// The codes are Crockford base32, which already excludes I, L, O and U for
// this reason; the remaining work is to apply Crockford's own decoding rules
// so a hand-written O read as an O still matches the 0 that was issued.
func normalizeCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'Z':
			switch r {
			case 'O':
				b.WriteRune('0')
			case 'I', 'L':
				b.WriteRune('1')
			default:
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// checkProvenance refuses evidence that cannot reach the tier the buyer asked
// for.
//
// V2 and above are defined as requiring capture provenance: the file has to
// say when and where it was taken. That metadata is attacker-supplied and
// weak on its own — which is exactly why it is treated as a floor rather than
// as proof. Without it the submission cannot be what the buyer bought, and
// accepting it anyway would make the tier a label rather than a standard.
func checkProvenance(sub *api.Submission) error {
	if !tierNeedsProvenance(sub.Tier) {
		return nil
	}
	// Where the job named an area, the location is the point.
	//
	// Accepting a timestamp in its place made the fence optional at the
	// submitter's discretion: a photograph carrying only a capture time
	// satisfied the tier, and checkGeofence then found nothing located and
	// returned clean. Somewhere else entirely would have passed.
	if sub.RadiusM > 0 {
		for _, a := range sub.Artifacts {
			if a.HasGeo {
				return nil
			}
		}
		return fmt.Errorf(
			"this job is tied to a place, so at least one photograph has to " +
				"record where it was taken. Turn on location for your camera and " +
				"take it again — a picture sent through a messaging app has that " +
				"stripped")
	}
	for _, a := range sub.Artifacts {
		if a.HasGeo || !a.CapturedAt.IsZero() {
			return nil
		}
	}
	return fmt.Errorf(
		"this job asks for %s, which needs photographs that record when and "+
			"where they were taken. Yours carry neither — turn on location for "+
			"your camera and take them again, and do not send them through a "+
			"messaging app, which strips it",
		sub.Tier)
}

// tierNeedsProvenance reports whether a tier's definition requires capture
// metadata.
func tierNeedsProvenance(tier string) bool {
	switch strings.ToUpper(strings.TrimSpace(tier)) {
	case "V2", "V3":
		return true
	default:
		return false
	}
}
