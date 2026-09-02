package exchange

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/account"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/media"
)

// Reference images: the buyer's half of making a job priceable.
//
// Two routes and no more. The buyer attaches an image before the job goes up;
// anybody looking at the open board can fetch it. That second half is the
// point — a reference nobody can see until they have already committed to the
// job is not a reference, it is a surprise.

func (s *Server) registerReferences(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/jobs/{job}/references", s.withBuyer(s.handleAddReference))
	// Deliberately unauthenticated, exactly like the board it belongs to.
	//
	// What is behind it is a photograph the buyer chose to publish so that
	// strangers could price their work; requiring a login to see it would mean
	// only people who had already signed up could decide whether to. The
	// address is still absent, and an image is only reachable if it is
	// attached to a listing that is itself public.
	mux.HandleFunc("GET /v1/jobs/{job}/references/{sha}", s.handleReferenceFile)
}

// handleAddReference stores an image and attaches it to a job.
func (s *Server) handleAddReference(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	job := r.PathValue("job")
	l, ok := s.ownedBy(w, job, person)
	if !ok {
		return
	}
	if len(body) == 0 {
		writeError(w, http.StatusBadRequest, "send the image as the request body")
		return
	}
	// Sniffed from the bytes, not taken from a header. The caption is the
	// buyer's to write; what the file *is* is the file's to say.
	mime := http.DetectContentType(body)
	if media.KindOf(mime) != media.KindImage {
		writeError(w, http.StatusUnsupportedMediaType,
			"a reference has to be an image — a photograph of the site, the "+
				"access, or the number on the door")
		return
	}
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])

	ref := api.Reference{
		SHA256: sha, Mime: mime, Bytes: len(body),
		Caption:    strings.TrimSpace(r.URL.Query().Get("caption")),
		Identifies: r.URL.Query().Get("identifies") == "true",
	}
	probe := *l
	probe.References = append(append([]api.Reference(nil), l.References...), ref)
	if err := probe.ValidateReferences(); err != nil {
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "board: "))
		return
	}
	if err := s.AddReference(job, body, mime, ref.Caption, ref.Identifies); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	after, _ := s.Board.Get(job)
	writeJSONResponse(w, map[string]any{
		"job": job, "sha256": sha, "bytes": len(body),
		"references": len(after.References),
		"url":        "/v1/jobs/" + job + "/references/" + sha,
		"note": "published with the job, so anybody deciding whether to bid can " +
			"see it before they commit",
	})
}

// handleReferenceFile serves a published reference.
func (s *Server) handleReferenceFile(w http.ResponseWriter, r *http.Request) {
	job := r.PathValue("job")
	sha := r.PathValue("sha")
	l, ok := s.Board.Get(job)
	if !ok {
		writeError(w, http.StatusNotFound, "no such job")
		return
	}
	// Directed work is not on the open board, so its references are not
	// public either — publishing them would say who this buyer works with.
	if l.Directed() {
		writeError(w, http.StatusNotFound, "no such job")
		return
	}
	// The image must be attached to this job. Otherwise the route is a
	// content-addressed read of every blob on the exchange, and the hash of
	// somebody's evidence is all it would take.
	var found bool
	for _, ref := range l.References {
		if ref.SHA256 == sha {
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "no such reference on this job")
		return
	}
	data, ok := s.blobFor(sha)
	if !ok {
		writeError(w, http.StatusGone, "that image is no longer stored")
		return
	}
	mime := s.mimeFor(sha)
	if mime == "" {
		mime = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeContent(w, r, sha, time.Time{}, bytes.NewReader(data))
}

// AddReference stores an image and attaches it to a listing.
//
// The route and the demo seeder both go through here, so a reference created
// by either behaves identically — same validation, same blob store, same
// public URL. A seeder with its own path is a seeder that drifts.
func (s *Server) AddReference(job string, data []byte, mime, caption string, identifies bool) error {
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	if err := s.Board.AttachReference(job, api.Reference{
		SHA256: sha, Mime: mime, Bytes: len(data),
		Caption: caption, Identifies: identifies,
	}); err != nil {
		return err
	}
	s.putBlob(sha, mime, data)
	return nil
}
