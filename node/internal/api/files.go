package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Files and images in a channel.
//
// A file is stored once in the account of whoever attached it, named by the
// SHA-256 of its bytes, and a message carries a small reference to it: name,
// type, size, and the address it is served from. The agent reads recent
// attachments (images and PDFs go to the model as they are; text inline).
//
// On a host, the address is a capability: /f/<account>/<id>/<sig>, signed by
// the host, so everyone the channel is shared with can open what is attached
// to it, and nobody can guess their way to anything else. On a laptop the
// owner's own route serves it.

const maxFileBytes = 20 << 20

// FileRef is what a message says about an attachment.
type FileRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

var allowedTypes = map[string]string{
	"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp",
	"application/pdf": ".pdf", "text/plain": ".txt", "text/markdown": ".md", "text/csv": ".csv",
	"application/json": ".json",
}

func filesDir(dataDir string) string { return filepath.Join(dataDir, "files") }

// fileType settles what a file is from its bytes, falling back to its name
// for the text kinds that sniff as plain text.
func fileType(name string, head []byte) string {
	t := http.DetectContentType(head)
	if i := strings.Index(t, ";"); i >= 0 {
		t = t[:i]
	}
	if t == "text/plain" || t == "application/octet-stream" {
		switch strings.ToLower(filepath.Ext(name)) {
		case ".md", ".markdown":
			return "text/markdown"
		case ".csv":
			return "text/csv"
		case ".json":
			return "application/json"
		case ".txt", ".log":
			return "text/plain"
		}
	}
	return t
}

func cleanName(n string) string {
	n = filepath.Base(strings.TrimSpace(n))
	n = strings.Map(func(r rune) rune {
		if r < 32 || r == '/' || r == '\\' || r == '"' {
			return -1
		}
		return r
	}, n)
	if n == "" || n == "." {
		n = "file"
	}
	if len(n) > 120 {
		n = n[:120]
	}
	return n
}

// POST /app/api/upload?name=… with the file as the body -> FileRef.
func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	name := cleanName(r.URL.Query().Get("name"))
	body := http.MaxBytesReader(w, r.Body, maxFileBytes+1)
	data, err := io.ReadAll(body)
	if err != nil || len(data) > maxFileBytes {
		writeStatusJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "Files can be up to 20 MB."})
		return
	}
	if len(data) == 0 {
		writeStatusJSON(w, http.StatusBadRequest, map[string]any{"error": "That file is empty."})
		return
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	typ := fileType(name, head)
	ext, ok := allowedTypes[typ]
	if !ok {
		writeStatusJSON(w, http.StatusUnsupportedMediaType, map[string]any{"error": "Lamdis takes images (PNG, JPEG, GIF, WebP), PDFs and text files."})
		return
	}
	sum := sha256.Sum256(data)
	id := hex.EncodeToString(sum[:16])
	dir := filesDir(a.DataDir)
	os.MkdirAll(dir, 0o700)
	if err := os.WriteFile(filepath.Join(dir, id+ext), data, 0o600); err != nil {
		writeStatusJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not keep the file"})
		return
	}
	ref := FileRef{ID: id, Name: name, Type: typ, Size: int64(len(data)), URL: a.fileURL(id)}
	meta, _ := json.Marshal(ref)
	os.WriteFile(filepath.Join(dir, id+".json"), meta, 0o600)
	writeJSON(w, ref)
}

func (a *App) fileURL(id string) string {
	if a.FileURL != nil {
		return a.FileURL(id)
	}
	return "/app/api/file/" + id
}

// fileRefs resolves the ids a message names to their references, dropping
// any this account does not hold.
func (a *App) fileRefs(ids []string) []FileRef {
	var out []FileRef
	for _, id := range ids {
		if !isFileID(id) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(filesDir(a.DataDir), id+".json"))
		if err != nil {
			continue
		}
		var f FileRef
		if json.Unmarshal(raw, &f) == nil {
			f.URL = a.fileURL(id)
			out = append(out, f)
		}
		if len(out) == 10 {
			break
		}
	}
	return out
}

func isFileID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// GET /app/api/file/{id} — the owner's own route.
func (a *App) handleFile(w http.ResponseWriter, r *http.Request) {
	serveFile(w, r, a.DataDir, r.PathValue("id"))
}

// serveFile sends one stored file, as itself and never as a page.
func serveFile(w http.ResponseWriter, r *http.Request, dataDir, id string) {
	if !isFileID(id) {
		http.NotFound(w, r)
		return
	}
	raw, err := os.ReadFile(filepath.Join(filesDir(dataDir), id+".json"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var f FileRef
	json.Unmarshal(raw, &f)
	ext, ok := allowedTypes[f.Type]
	if !ok {
		http.NotFound(w, r)
		return
	}
	data, err := os.ReadFile(filepath.Join(filesDir(dataDir), id+ext))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	disp := "inline"
	if !strings.HasPrefix(f.Type, "image/") && f.Type != "application/pdf" {
		disp = "attachment"
	}
	w.Header().Set("Content-Type", f.Type)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": f.Name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(data)
}

// fileURL signs an attachment's address on a host. Whoever holds it can
// open that one file; it names no other.
func (h *Host) fileURL(acct, id string) string {
	return "/f/" + acct + "/" + id + "/" + h.fileSig(acct, id)
}

func (h *Host) fileSig(acct, id string) string {
	secret, err := h.guestSecret()
	if err != nil {
		return "x"
	}
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("file\x00" + acct + "\x00" + id))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))[:22]
}

// GET /f/{acct}/{id}/{sig}
func (h *Host) handleSharedFile(w http.ResponseWriter, r *http.Request) {
	acct, id, sig := r.PathValue("acct"), r.PathValue("id"), r.PathValue("sig")
	if strings.ContainsAny(acct, "./\\") || !hmac.Equal([]byte(sig), []byte(h.fileSig(acct, id))) {
		http.NotFound(w, r)
		return
	}
	if _, err := os.Stat(filepath.Join(h.Root, ".deleted", acct)); err == nil {
		http.NotFound(w, r)
		return
	}
	serveFile(w, r, filepath.Join(h.Root, acct), id)
}

// openFile lets an account's agent read an attachment from any account on
// this host, but only through a correctly signed address: the same thing
// the person could open in the channel.
func (h *Host) openFile(url, id string) ([]byte, error) {
	p := strings.Split(strings.TrimPrefix(url, "/f/"), "/")
	if !strings.HasPrefix(url, "/f/") || len(p) != 3 || p[1] != id || !isFileID(id) ||
		strings.ContainsAny(p[0], "./\\") || !hmac.Equal([]byte(p[2]), []byte(h.fileSig(p[0], id))) {
		return nil, os.ErrNotExist
	}
	dir := filesDir(filepath.Join(h.Root, p[0]))
	raw, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return nil, err
	}
	var f FileRef
	json.Unmarshal(raw, &f)
	ext, ok := allowedTypes[f.Type]
	if !ok {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(filepath.Join(dir, id+ext))
}
