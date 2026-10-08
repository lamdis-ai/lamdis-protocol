package agent

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// What the harness does to keep a change honest after the model has made
// it: tell the change's failures apart from the project's, and never leave
// a file it cannot parse behind.

// original is a changed file as it was before this task: what the harness
// saved before the first edit, or, for a file a command changed, what git
// has committed, provided the file was clean when the task began. ok is
// false when the original cannot be known; content is nil when the file did
// not exist.
func (h *harness) original(ctx context.Context, abs string) (content []byte, ok bool) {
	if orig, known := h.originals[abs]; known {
		return orig, true
	}
	if !h.git {
		return nil, false
	}
	if _, dirty := h.baseline[abs]; dirty {
		return nil, false // it already differed from git before the task
	}
	top, err := gitRaw(ctx, h.root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, false
	}
	rel, err := filepath.Rel(resolveSymlinks(strings.TrimSpace(top)), abs)
	if err != nil {
		return nil, false
	}
	out, err := gitRaw(ctx, h.root, "show", "HEAD:"+filepath.ToSlash(rel))
	if err != nil {
		return nil, true // clean and absent from HEAD: the task created it
	}
	return []byte(out), true
}

// swapJournal is where the change's own version of each file is kept while
// the originals are put back for a comparison run. If the process dies in
// between, the next task in this workspace puts the change back first.
func (h *harness) swapJournal() string {
	if h.r == nil || h.r.DataDir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(h.root))
	return filepath.Join(h.r.DataDir, "swap", hex.EncodeToString(sum[:8])+".json")
}

// recoverSwap restores a change left swapped out by a run that died while
// comparing against the originals. It reports what it put back.
func (h *harness) recoverSwap() []string {
	path := h.swapJournal()
	raw, err := os.ReadFile(path)
	if path == "" || err != nil {
		return nil
	}
	var j map[string]*string
	if json.Unmarshal(raw, &j) != nil {
		return nil
	}
	var put []string
	for p, b64 := range j {
		if !strings.HasPrefix(p, h.root) {
			continue
		}
		if b64 == nil {
			os.Remove(p)
		} else if body, err := base64.StdEncoding.DecodeString(*b64); err == nil {
			os.WriteFile(p, body, fileMode(p))
		}
		put = append(put, p)
	}
	os.Remove(path)
	return put
}

func fileMode(p string) os.FileMode {
	if st, err := os.Stat(p); err == nil {
		return st.Mode().Perm()
	}
	return 0o644
}

// withOriginals runs fn with every changed file put back as it was before
// the task, and the change restored afterwards, journalled so a crash in
// between loses nothing. It does nothing and reports false when any file's
// original cannot be known.
func (h *harness) withOriginals(ctx context.Context, files []string, fn func()) bool {
	type put struct {
		path string
		orig []byte
	}
	var puts []put
	for _, f := range files {
		orig, ok := h.original(ctx, f)
		if !ok {
			return false
		}
		puts = append(puts, put{f, orig})
	}
	journal := map[string]*string{}
	for _, p := range puts {
		if now, err := os.ReadFile(p.path); err == nil {
			s := base64.StdEncoding.EncodeToString(now)
			journal[p.path] = &s
		} else {
			journal[p.path] = nil
		}
	}
	jpath := h.swapJournal()
	if jpath == "" {
		return false
	}
	raw, _ := json.Marshal(journal)
	os.MkdirAll(filepath.Dir(jpath), 0o700)
	if err := os.WriteFile(jpath, raw, 0o600); err != nil {
		return false
	}
	defer h.recoverSwap()
	for _, p := range puts {
		if p.orig == nil {
			os.Remove(p.path)
		} else {
			os.WriteFile(p.path, p.orig, fileMode(p.path))
		}
	}
	fn()
	return true
}

var volatile = regexp.MustCompile(`\b\d+(\.\d+)?\s?(ms|s|sec|secs|seconds)\b|0x[0-9a-fA-F]+|\bin \d+(\.\d+)?s\b`)

// newFailures is what fails with the change that did not fail without it.
func newFailures(now, before checkResult) []string {
	if before.Status == "passed" {
		return now.Failures
	}
	seen := map[string]bool{}
	for _, f := range before.Failures {
		seen[volatile.ReplaceAllString(f, "")] = true
	}
	var out []string
	for _, f := range now.Failures {
		if !seen[volatile.ReplaceAllString(f, "")] {
			out = append(out, f)
		}
	}
	return out
}

// statusFailingBefore marks a check that fails the same way without the
// change: the project's problem, reported, and not the change's to repair.
const statusFailingBefore = "failing_before"

// judge decides whether a failing check is the change's fault, by running
// it again on the original files. A check that also fails there, with no
// failure the change added, is marked failing_before.
func (h *harness) judge(ctx context.Context, files []string, res checkResult) checkResult {
	if res.Status != "failed" && res.Status != "timed_out" {
		return res
	}
	var before checkResult
	if !h.withOriginals(ctx, files, func() { before = runCheck(ctx, res.check) }) || ctx.Err() != nil {
		return res
	}
	switch {
	case before.Status == "unavailable":
		return res
	case before.Status == "passed":
		return res // the change broke it
	case res.Status == "timed_out" && before.Status == "timed_out":
		res.Status = statusFailingBefore
	case len(newFailures(res, before)) == 0:
		res.Status = statusFailingBefore
	default:
		// Show the model only what it caused, then what was already there.
		fresh := newFailures(res, before)
		res.Failures = append(fresh, "(already failing before this change: "+fmt.Sprint(len(res.Failures)-len(fresh))+" more lines)")
	}
	return res
}

// syntaxCheck is a fast, dependency-free parse of one file, or "" when the
// language has none the harness can run.
func syntaxCheck(path string) []string {
	switch filepath.Ext(path) {
	case ".py":
		// -I: the file's own directory may hold modules that shadow the
		// standard library (a package's warnings.py), which would break
		// the compiler itself rather than report on the file.
		return []string{"python3", "-I", "-m", "py_compile", path}
	case ".go":
		return []string{"gofmt", "-e", "-l", path}
	case ".js", ".mjs", ".cjs":
		return []string{"node", "--check", path}
	}
	return nil
}

// parses reports whether a file parses, and why not. A checker that is not
// installed counts as parsing: the guard only acts on evidence.
func parses(ctx context.Context, path string) (bool, string) {
	if filepath.Ext(path) == ".json" {
		raw, err := os.ReadFile(path)
		if err != nil || json.Valid(raw) {
			return true, ""
		}
		return false, "not valid JSON"
	}
	argv := syntaxCheck(path)
	if argv == nil {
		return true, ""
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		return true, ""
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, argv[0], argv[1:]...)
	cmd.Dir = os.TempDir() // never the project, whose modules could shadow the checker's
	out, err := cmd.CombinedOutput()
	if err != nil && cctx.Err() == nil {
		return false, strings.TrimSpace(lastLines(string(out), 3))
	}
	return true, ""
}

func lastLines(s string, n int) string {
	ls := strings.Split(strings.TrimSpace(s), "\n")
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	return strings.Join(ls, " | ")
}

// guardSyntax runs when a task ends, however it ends. A file the change
// left unparseable, when it parsed before, is put back as it was: a run
// that stops part-way through an edit must not leave the project unable to
// import. What it put back goes into the report as unresolved.
func (h *harness) guardSyntax(ctx context.Context) {
	for _, f := range h.changed(ctx) {
		if _, err := os.Stat(f); err != nil {
			continue
		}
		ok, why := parses(ctx, f)
		if ok {
			continue
		}
		orig, known := h.original(ctx, f)
		if !known {
			h.unresolved = appendOnce(h.unresolved, h.ws.rel(f)+" does not parse ("+trunc(why, 200)+")")
			continue
		}
		if orig != nil {
			tmp, err := os.CreateTemp("", "lamdis-parse-*"+filepath.Ext(f))
			if err != nil {
				continue
			}
			tmp.Write(orig)
			tmp.Close()
			origOK, _ := parses(ctx, tmp.Name())
			os.Remove(tmp.Name())
			if !origOK {
				h.unresolved = appendOnce(h.unresolved, h.ws.rel(f)+" does not parse, and did not before this task either")
				continue
			}
			os.WriteFile(f, orig, fileMode(f))
		} else {
			os.Remove(f)
		}
		h.unresolved = appendOnce(h.unresolved, "put "+h.ws.rel(f)+" back as it was, because the change left it unparseable ("+trunc(why, 200)+")")
	}
}
