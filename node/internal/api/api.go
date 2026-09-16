// Package api is the node's HTTP surface. v1 carries the sync protocol
// (peer-to-peer and client↔hub use the same endpoints). Requests are
// authenticated by Ed25519 request signatures: the caller IS a principal;
// there is no ambient authority anywhere.
package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	syncp "github.com/lamdis-ai/lamdis-protocol/node/internal/sync"
)

const (
	hdrPrincipal = "X-Lamdis-Principal"
	hdrTimestamp = "X-Lamdis-Timestamp"
	hdrSignature = "X-Lamdis-Signature"
	maxSkew      = 5 * time.Minute
	maxBody      = 32 << 20 // 32 MiB per request
)

// signingInput binds method, path, time, and body so a signature cannot be
// replayed against another endpoint or payload.
func signingInput(method, path, timestamp string, body []byte) []byte {
	sum := sha256.Sum256(body)
	return []byte(method + "\n" + path + "\n" + timestamp + "\n" + hex.EncodeToString(sum[:]))
}

// Sign adds auth headers to an outgoing request whose body is body.
func Sign(req *http.Request, priv ed25519.PrivateKey, body []byte) error {
	pid, err := protolog.PrincipalID(priv.Public().(ed25519.PublicKey))
	if err != nil {
		return err
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	sig := ed25519.Sign(priv, signingInput(req.Method, req.URL.Path, ts, body))
	req.Header.Set(hdrPrincipal, pid)
	req.Header.Set(hdrTimestamp, ts)
	req.Header.Set(hdrSignature, hex.EncodeToString(sig))
	return nil
}

// authenticate verifies the request signature and returns the principal id.
func authenticate(r *http.Request, body []byte, now time.Time) (string, error) {
	pid := r.Header.Get(hdrPrincipal)
	ts := r.Header.Get(hdrTimestamp)
	sigHex := r.Header.Get(hdrSignature)
	if pid == "" || ts == "" || sigHex == "" {
		return "", fmt.Errorf("missing auth headers")
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "", fmt.Errorf("bad timestamp")
	}
	if d := now.Sub(t); d > maxSkew || d < -maxSkew {
		return "", fmt.Errorf("timestamp outside allowed skew")
	}
	pub, err := protolog.PublicKey(pid)
	if err != nil {
		return "", err
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return "", fmt.Errorf("bad signature encoding")
	}
	if !ed25519.Verify(pub, signingInput(r.Method, r.URL.Path, ts, body), sig) {
		return "", fmt.Errorf("signature verification failed")
	}
	return pid, nil
}

// Server exposes the sync protocol over HTTP.
type Server struct {
	// Prefix mounts this node's sync API somewhere other than the root,
	// which is how one process serves many nodes. It is part of the path a
	// caller signs, so it cannot be stripped before the signature is
	// checked; the routes carry it instead.
	Prefix string
	Sync   *syncp.Server
	// Principal is this node's person principal id, served unauthenticated
	// at /v1/node so pairing can exchange identities automatically.
	Principal string
	Now       func() time.Time
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) Handler() *http.ServeMux {
	mux := http.NewServeMux()
	p := strings.TrimRight(s.Prefix, "/")
	// Node identity is public by design: it's how two people pair. It reveals
	// nothing about threads or content.
	mux.HandleFunc("GET "+p+"/v1/node", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"principal": s.Principal})
	})
	mux.HandleFunc("POST "+p+"/v1/sync/list", s.withAuth(s.handleList))
	mux.HandleFunc("POST "+p+"/v1/sync/pull", s.withAuth(s.handlePull))
	mux.HandleFunc("POST "+p+"/v1/sync/push", s.withAuth(s.handlePush))
	mux.HandleFunc("POST "+p+"/v1/discover", s.withAuth(s.handleDiscover))
	mux.HandleFunc("POST "+p+"/v1/access/request", s.withAuth(s.handleAccessRequest))
	return mux
}

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request, principal string, _ []byte) {
	threads, err := s.Sync.Discover(r.Context(), principal)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"threads": threads})
}

func (s *Server) handleAccessRequest(w http.ResponseWriter, r *http.Request, principal string, body []byte) {
	var e protolog.Entry
	if err := json.Unmarshal(body, &e); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := s.Sync.SubmitAccessRequest(r.Context(), principal, &e); err != nil {
		http.Error(w, "rejected", http.StatusForbidden)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// FetchNodeInfo returns the principal a node at baseURL identifies as.
func FetchNodeInfo(ctx context.Context, baseURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/node", nil)
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("node info: %s", resp.Status)
	}
	var out struct {
		Principal string `json:"principal"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if _, err := protolog.PublicKey(out.Principal); err != nil {
		return "", fmt.Errorf("node returned an invalid principal: %w", err)
	}
	return out.Principal, nil
}

// WithAuth is the exported form, so other packages can mount routes behind the
// same Ed25519 request-signature check rather than inventing their own.
func (s *Server) WithAuth(next func(w http.ResponseWriter, r *http.Request, principal string, body []byte)) http.HandlerFunc {
	return s.withAuth(next)
}

func (s *Server) withAuth(next func(w http.ResponseWriter, r *http.Request, principal string, body []byte)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		principal, err := authenticate(r, body, s.now())
		if err != nil {
			// One generic message: no oracle about which check failed.
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r, principal, body)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request, principal string, _ []byte) {
	ids, err := s.Sync.List(r.Context(), principal)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"threads": ids})
}

func (s *Server) handlePull(w http.ResponseWriter, r *http.Request, principal string, body []byte) {
	var req syncp.PullRequest
	if err := json.Unmarshal(body, &req); err != nil || req.Thread == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	resp, err := s.Sync.Pull(r.Context(), principal, req)
	if err != nil {
		// Not-found and not-permitted are deliberately the same status.
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, resp)
}

func (s *Server) handlePush(w http.ResponseWriter, r *http.Request, principal string, body []byte) {
	var req syncp.PushRequest
	if err := json.Unmarshal(body, &req); err != nil || req.Thread == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	resp, err := s.Sync.Push(r.Context(), principal, req)
	if err != nil {
		// Same shape as pull: no oracle distinguishing missing/forbidden/invalid.
		http.Error(w, "rejected", http.StatusForbidden)
		return
	}
	writeJSON(w, resp)
}

// HTTPTransport implements sync.Transport against a remote node.
type HTTPTransport struct {
	BaseURL string
	Key     ed25519.PrivateKey
	Client  *http.Client
}

func NewHTTPTransport(baseURL string, key ed25519.PrivateKey) *HTTPTransport {
	return &HTTPTransport{BaseURL: baseURL, Key: key, Client: &http.Client{Timeout: 60 * time.Second}}
}

func (t *HTTPTransport) post(ctx context.Context, path string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := Sign(req, t.Key, body); err != nil {
		return err
	}
	resp, err := t.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s: %s: %s", path, resp.Status, bytes.TrimSpace(msg))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (t *HTTPTransport) List(ctx context.Context) ([]string, error) {
	var out struct {
		Threads []string `json:"threads"`
	}
	if err := t.post(ctx, "/v1/sync/list", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return out.Threads, nil
}

func (t *HTTPTransport) Pull(ctx context.Context, req syncp.PullRequest) (*syncp.PullResponse, error) {
	var out syncp.PullResponse
	if err := t.post(ctx, "/v1/sync/pull", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (t *HTTPTransport) Discover(ctx context.Context) ([]syncp.DiscoverableThread, error) {
	var out struct {
		Threads []syncp.DiscoverableThread `json:"threads"`
	}
	if err := t.post(ctx, "/v1/discover", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return out.Threads, nil
}

func (t *HTTPTransport) RequestAccess(ctx context.Context, e *protolog.Entry) error {
	var out map[string]any
	return t.post(ctx, "/v1/access/request", e, &out)
}

func (t *HTTPTransport) Push(ctx context.Context, req syncp.PushRequest) (*syncp.PushResponse, error) {
	var out syncp.PushResponse
	if err := t.post(ctx, "/v1/sync/push", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AuthenticatePrincipal verifies a request signed by a principal's own key.
//
// Exported because the exchange's buy-side routes need the same answer the
// node's own routes get: an integration holding a keypair is an account, and
// the routes that describe how it spends must accept what the route that
// spends already accepts.
func AuthenticatePrincipal(r *http.Request, body []byte, now time.Time) (string, error) {
	return authenticate(r, body, now)
}
