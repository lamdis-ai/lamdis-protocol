package exchange

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/account"
)

// Anchored receipts.
//
// A receipt is signed by this exchange, which proves the exchange issued it —
// to anybody who trusts the exchange. Anchoring adds the thing a signature
// cannot: proof that the receipt existed, in exactly this form, at a point in
// time, checkable by somebody who trusts neither Lamdis nor its continued
// existence. The hash of each receipt goes into a Merkle tree; the root is
// committed to Bitcoin through OpenTimestamps; the path from receipt to root
// and the .ots proof are handed out here.
//
// What the hash covers: the receipt object with its `signature` and `anchor`
// members removed, serialised compactly with keys sorted — which is what
// encoding/json produces for a map, and what `jq -cS 'del(.signature,.anchor)'`
// reproduces. The anchor pointer cannot be inside what it anchors; the
// signature is left out so re-signing an unchanged receipt does not change
// what was anchored.

// anchorReceipt is the one hook the receipt handler calls, before signing.
//
// Two things happen. First, an unchanged receipt is re-served with the issue
// time it was first served with, so serving it twice yields the same bytes
// and the same hash; without that every GET would be a new document and no
// receipt could ever carry its own proof. Second, the hash is recorded and a
// summary of its anchoring is attached under "anchor". Nothing here waits on
// the network.
func (s *Server) anchorReceipt(job string, out map[string]any) {
	if s.Anchors == nil {
		return
	}
	issued, _ := out["issued_at"].(string)
	delete(out, "issued_at")
	fp, ok := canonicalHash(out)
	if !ok {
		out["issued_at"] = issued
		return
	}
	if at, pinned := s.Anchors.IssuedAt(job, fp); pinned {
		issued = at
	}
	out["issued_at"] = issued
	sha, ok := canonicalHash(out)
	if !ok {
		return
	}
	s.Anchors.Record(job, fp, issued, sha)
	out["anchor"] = s.anchorSummary(job, sha)
}

// anchorSummary is the pointer a receipt carries: enough to find the proof
// and to see whether it is on the chain yet.
func (s *Server) anchorSummary(job, sha string) map[string]any {
	p, _ := s.Anchors.Proof(sha)
	sum := map[string]any{
		"receipt_sha256": sha,
		"status":         p.Status,
		"proof":          "/v1/jobs/" + job + "/receipt/anchor?sha256=" + sha,
		"method":         "opentimestamps",
	}
	if p.MerkleRoot != "" {
		sum["merkle_root"] = p.MerkleRoot
	}
	if p.BitcoinHeight > 0 {
		sum["bitcoin_block"] = p.BitcoinHeight
	}
	return sum
}

func canonicalHash(v map[string]any) (string, bool) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:]), true
}

// registerAnchors mounts the public list of roots. The per-receipt proof is
// mounted beside the receipt, under the same credential.
func (s *Server) registerAnchors(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/anchors", s.handleAnchors)
}

// handleAnchors lists recent roots and their status. Public: a root is a
// hash of hashes and reveals nothing, and the point is that a stranger can
// check the chain of them without asking anybody's permission.
func (s *Server) handleAnchors(w http.ResponseWriter, r *http.Request) {
	if s.Anchors == nil {
		writeJSONResponse(w, map[string]any{
			"enabled": false,
			"note":    "this exchange is not anchoring receipts; run it with a data directory and -anchor",
		})
		return
	}
	batches := s.Anchors.Batches(50)
	out := make([]map[string]any, 0, len(batches))
	for _, b := range batches {
		e := map[string]any{
			"seq": b.Seq, "merkle_root": b.Root, "status": b.Status(),
			"created_at": b.CreatedAt,
		}
		if b.SubmittedAt != nil {
			e["submitted_at"] = b.SubmittedAt
			e["calendars"] = b.Calendars
			e["ots_proof"] = b.OTS
		}
		if b.AnchoredAt != nil {
			e["anchored_at"] = b.AnchoredAt
			e["bitcoin_block"] = b.BitcoinHeight
		}
		out = append(out, e)
	}
	writeJSONResponse(w, map[string]any{
		"enabled":       true,
		"method":        "opentimestamps",
		"calendars":     s.Anchors.Calendars(),
		"every":         s.Anchors.Every().String(),
		"unbatched":     s.Anchors.Pending(),
		"batches":       out,
		"how_to_verify": howToVerifyRoot,
	})
}

// handleReceiptAnchor returns the proof for one receipt. Same credential as
// the receipt itself: whoever may read the receipt may read its proof.
func (s *Server) handleReceiptAnchor(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	job := r.PathValue("job")
	if _, ok := s.ownedBy(w, job, person); !ok {
		return
	}
	if s.Anchors == nil {
		writeError(w, http.StatusNotFound, "this exchange is not anchoring receipts")
		return
	}
	sha := r.URL.Query().Get("sha256")
	if sha == "" {
		latest, ok := s.Anchors.Latest(job)
		if !ok {
			writeError(w, http.StatusNotFound,
				"no receipt has been issued for this job yet; fetch the receipt first")
			return
		}
		sha = latest
	}
	p, ok := s.Anchors.Proof(sha)
	if !ok || p.Job != job {
		writeError(w, http.StatusNotFound, "no receipt of this job with that hash was issued here")
		return
	}
	out := map[string]any{
		"job":            job,
		"receipt_sha256": p.ReceiptSHA256,
		"recorded_at":    p.RecordedAt,
		"status":         p.Status,
		"method":         "opentimestamps",
		"calendars":      s.Anchors.Calendars(),
		"how_to_verify":  howToVerifyReceipt,
	}
	if p.MerkleRoot != "" {
		out["merkle_root"] = p.MerkleRoot
		out["inclusion_path"] = p.InclusionPath
	}
	if len(p.OTSProof) > 0 {
		// []byte marshals as base64.
		out["ots_proof"] = p.OTSProof
		out["accepted_by"] = p.Calendars
		out["submitted_at"] = p.SubmittedAt
	}
	if p.AnchoredAt != nil {
		out["anchored_at"] = p.AnchoredAt
		out["bitcoin_block"] = p.BitcoinHeight
	}
	if p.Note != "" {
		out["note"] = p.Note
	}
	writeJSONResponse(w, out)
}

var howToVerifyReceipt = []string{
	"1. Take the receipt JSON you saved. Delete its `signature` and `anchor` members, serialise it compactly with keys sorted (jq -cS 'del(.signature,.anchor)' receipt.json | tr -d '\\n'), and SHA-256 the bytes. It must equal receipt_sha256.",
	"2. Fold that hash up inclusion_path: at each step, SHA-256(sibling || hash) when side is \"left\", SHA-256(hash || sibling) when side is \"right\". The result must equal merkle_root.",
	"3. Decode ots_proof from base64 into root.ots. Install the OpenTimestamps client (pip install opentimestamps-client) and run: ots verify -d <merkle_root> root.ots. It reports the Bitcoin block that commits to the root and the time of that block.",
	"4. If status is pending, run `ots upgrade root.ots` first; the calendars will have a Bitcoin attestation within hours of submission. Nothing in steps 1-3 asks this exchange anything.",
	"What this proves: the receipt existed, byte for byte, no later than that block. What it does not prove: that what the receipt says is true — for that, read the receipt's verification block and its evidence.",
}

var howToVerifyRoot = []string{
	"Each batch is a Merkle root over the SHA-256 hashes of receipts served in that interval, submitted to the calendars listed. Decode ots_proof from base64 into root.ots and run: ots verify -d <merkle_root> root.ots (pip install opentimestamps-client).",
	"A pending batch has been accepted by a calendar and not yet committed to Bitcoin; `ots upgrade root.ots` fetches the attestation once it exists.",
	"A receipt's own proof, with its inclusion path to one of these roots, is at GET /v1/jobs/{job}/receipt/anchor for whoever may read the receipt.",
}
