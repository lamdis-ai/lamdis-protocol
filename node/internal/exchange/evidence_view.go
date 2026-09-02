package exchange

import (
	"bytes"
	"net/http"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/account"
)

// Letting the buyer look at what they paid for.
//
// Until this existed, a buyer could learn that a job was verified, and could
// read the SHA-256 of the photograph that proved it, and could do nothing
// else. Asking someone to trust a hash of a picture they are not allowed to
// see is asking them to take the exchange's word — which is precisely the
// thing the exchange exists to replace.
//
// The hash is what makes the evidence checkable. The bytes are what make it
// evidence.

func (s *Server) handleJobEvidence(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	job := r.PathValue("job")
	if _, ok := s.ownedBy(w, job, person); !ok {
		return
	}
	var files []map[string]any
	for _, sub := range s.Submissions(job) {
		for _, a := range sub.Artifacts {
			f := map[string]any{
				"sha256": a.SHA256, "mime": a.Mime, "kind": a.Kind,
				"bytes": a.Bytes, "at": sub.At.Format(time.RFC3339),
				"verified": sub.Verified,
				"view":     s.BaseURL + "/v1/jobs/" + job + "/evidence/" + a.SHA256,
			}
			// Say where the file claims it was taken and whether the challenge
			// code was found. A buyer judging the evidence needs the same
			// signals the verifier used, not just its conclusion.
			if a.HasGeo {
				f["lat"] = float64(a.LatE7) / 1e7
				f["lon"] = float64(a.LonE7) / 1e7
			}
			if !a.CapturedAt.IsZero() {
				f["captured_at"] = a.CapturedAt.Format(time.RFC3339)
			}
			if a.ChallengeSeen != "" {
				f["challenge_seen"] = a.ChallengeSeen
			}
			if a.Transcript != "" {
				f["transcript"] = a.Transcript
			}
			// What the checks thought of the imagery itself. Shown because a
			// buyer weighing a hold deserves the signals, not only the verdict
			// — and because one of these gates nothing yet, so a human is the
			// only thing currently reading it.
			if sub.Signals != nil {
				f["looks_generated"] = sub.Signals.Synthetic
				f["looks_like_a_screen_or_print"] = sub.Signals.Recapture
				if sub.Signals.InstructionLike {
					f["text_aimed_at_the_checker"] = true
				}
			}
			files = append(files, f)
		}
	}
	if files == nil {
		files = []map[string]any{}
	}
	writeJSONResponse(w, map[string]any{"job": job, "files": files})
}

func (s *Server) handleEvidenceFile(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	job := r.PathValue("job")
	if _, ok := s.ownedBy(w, job, person); !ok {
		return
	}
	sha := r.PathValue("sha")

	// The file must belong to this job. Without this check the route is a
	// content-addressed read of every blob on the exchange for anyone holding
	// any job — the hash of someone else's evidence is all it would take.
	var found bool
	for _, sub := range s.Submissions(job) {
		for _, a := range sub.Artifacts {
			if a.SHA256 == sha {
				found = true
			}
			for _, f := range a.Frames {
				if f == sha {
					found = true
				}
			}
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "no such file on this job")
		return
	}

	data, ok := s.blobFor(sha)
	if !ok {
		// With a data directory the bytes are on the durable disk and this is
		// rare; without one they died with the last process. Either way the
		// hash and the verdict survive, so say that plainly rather than
		// returning a 404 that reads as "this never existed".
		writeError(w, http.StatusGone,
			"the file is no longer stored; its hash and verdict remain on the receipt")
		return
	}

	mime := s.mimeFor(sha)
	if mime == "" {
		mime = "application/octet-stream"
	}
	// A hostile upload must not be able to run in the exchange's origin.
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeContent(w, r, sha, time.Time{}, bytes.NewReader(data))
}
