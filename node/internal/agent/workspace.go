package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// A workspace is a directory the agent may work in: a repository, usually.
// The tools are few on purpose. gdf's measurements were unambiguous that a
// harness gets fast by shipping a handful of tools with a byte-stable prompt
// and by distilling what comes back, not by choosing a language. Six tools,
// all bounded, every path checked to stay inside the root.

// A workspace is one or more places the agent may work. It starts with
// wherever you launched it and grows only when you say so: the agent asks
// for a directory the way it asks for any other decision, and you answer
// from wherever you happen to be.
type Workspace struct {
	Root string
	// Roots are the directories it may reach, Root included. Empty means
	// Root alone.
	Roots []string
	// Ask requests a directory. It returns true when the person allowed it,
	// and is nil where nobody can be asked, which means the answer is no.
	Ask func(ctx context.Context, path, why string) (bool, error)
	// Remember persists a directory somebody allowed, so the same question
	// is never asked twice.
	Remember func(path string)
	// Guard refuses credential stores even inside allowed ground. On unless
	// somebody has deliberately turned it off.
	Guard bool

	mu sync.Mutex
}

// roots is every place it may currently work.
func (w *Workspace) roots() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := []string{w.Root}
	out = append(out, w.Roots...)
	return out
}

// Allow adds a directory to what the agent may reach.
func (w *Workspace) Allow(dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	abs = resolveSymlinks(abs)
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, r := range append([]string{w.Root}, w.Roots...) {
		if r == abs {
			return
		}
	}
	w.Roots = append(w.Roots, abs)
}

// Allowed reports whether a path is inside somewhere already permitted.
func (w *Workspace) Allowed(abs string) bool {
	for _, r := range w.roots() {
		root, err := filepath.EvalSymlinks(r)
		if err != nil {
			root = r
		}
		if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

const (
	readMaxBytes = 200_000
	runTimeout   = 120 * time.Second
	runHeadLines = 80
	runTailLines = 80
	runMaxChars  = 12_000
	mapMaxFiles  = 400
)

var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, "target": true, "dist": true,
	"out": true, ".next": true, "build": true, "__pycache__": true, ".venv": true, "venv": true, ".idea": true, ".vscode": true}

// resolve keeps every path under the root. A symlink that points outside is
// refused too, since the agent's reach is the directory, not the filesystem.
func (w *Workspace) resolve(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path is required")
	}
	root, err := filepath.EvalSymlinks(w.Root)
	if err != nil {
		root = w.Root
	}
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, p)
	}
	abs = filepath.Clean(abs)
	abs = resolveSymlinks(abs)
	if w.Guard {
		if bad, what := Guarded(abs); bad {
			return "", fmt.Errorf("%s is in %s, which holds credentials. That is off limits even here; ask the person to fetch what you need", p, what)
		}
	}
	if !w.Allowed(abs) {
		return "", fmt.Errorf("%s is somewhere you have not allowed. Use open_path to ask for it", p)
	}
	return abs, nil
}

// resolveSymlinks follows links as far as it can. A file that does not
// exist yet cannot be resolved, but the directory it would go in can, and
// that is what decides whether it is somewhere allowed. On a Mac this is
// not a nicety: /var is a link to /private/var, so without it writing a new
// file into a directory somebody just allowed would be refused.
func resolveSymlinks(abs string) string {
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	dir, base := filepath.Split(abs)
	if dir == "" || dir == abs {
		return abs
	}
	if real, err := filepath.EvalSymlinks(filepath.Clean(dir)); err == nil {
		return filepath.Join(real, base)
	}
	return abs
}

// homeDir is where a person's own things live, and the one place a request
// for "everything" is always refused: an agent that may read your whole home
// directory may read your keys, your mail and your browser profile.
func homeDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Clean(h)
}

// RequestPath asks the person for a directory, and adds it if they agree.
func (w *Workspace) RequestPath(ctx context.Context, dir, why string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return "", fmt.Errorf("that is not a path")
	}
	abs = filepath.Clean(abs)
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("%s is not there", dir)
	}
	if !st.IsDir() {
		abs = filepath.Dir(abs)
	}
	abs = resolveSymlinks(abs)
	if w.Guard {
		if bad, what := Guarded(abs); bad {
			return "", fmt.Errorf("%s holds credentials, so it is not offered", what)
		}
	}
	if w.Allowed(abs) {
		return abs, nil
	}
	// Some requests are refused rather than asked, because no answer given
	// in a hurry should be able to hand over everything at once.
	switch {
	case abs == "/" || abs == filepath.VolumeName(abs)+string(filepath.Separator):
		return "", fmt.Errorf("the whole filesystem is not something to hand over; ask for the directory you need")
	case abs == homeDir():
		return "", fmt.Errorf("your home directory holds keys, mail and browser profiles, so it is not offered whole; ask for the folder you need")
	}
	if w.Ask == nil {
		return "", fmt.Errorf("nobody is here to allow that")
	}
	ok, err := w.Ask(ctx, abs, why)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("they said no")
	}
	w.Allow(abs)
	if w.Remember != nil {
		w.Remember(abs)
	}
	return abs, nil
}

// rel names a file the shortest way that is still unambiguous: relative to
// the place you launched in, absolute anywhere else.
func (w *Workspace) rel(abs string) string {
	root, err := filepath.EvalSymlinks(w.Root)
	if err != nil {
		root = w.Root
	}
	if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
		if r, err := filepath.Rel(root, abs); err == nil {
			return r
		}
	}
	return abs
}

// Map is the repository at a glance: paths and sizes, capped. It goes at
// the start of the prompt once per session so the prefix stays stable.
func (w *Workspace) Map() string {
	var files []string
	sizes := map[string]int64{}
	n := 0
	for _, root := range w.roots() {
		w.mapInto(root, &files, sizes, &n)
	}
	sort.Strings(files)
	var sb strings.Builder
	for _, f := range files {
		fmt.Fprintf(&sb, "%s (%s)\n", f, humanSize(sizes[f]))
	}
	if n >= mapMaxFiles {
		sb.WriteString("… (more files not listed; use list_files or search_files)\n")
	}
	return sb.String()
}

func (w *Workspace) mapInto(root string, files *[]string, sizes map[string]int64, n *int) {
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if *n >= mapMaxFiles {
			return filepath.SkipAll
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		r := w.rel(p)
		*files = append(*files, r)
		sizes[r] = info.Size()
		*n++
		return nil
	})
}

func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.0fK", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1024/1024)
	}
}

// Specs are the workspace tools, in the OpenAI function shape.
func (w *Workspace) Specs() []ToolSpec {
	obj := func(props map[string]any, req ...string) map[string]any {
		m := map[string]any{"type": "object", "properties": props}
		if len(req) > 0 {
			m["required"] = req
		}
		return m
	}
	str := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	num := func(d string) map[string]any { return map[string]any{"type": "integer", "description": d} }
	return []ToolSpec{
		{Name: "read_file", Description: "Read a file, or a line range of it. Paths are relative to the workspace.",
			Parameters: obj(map[string]any{"path": str("file path"), "start": num("first line, 1-based (optional)"), "end": num("last line, inclusive (optional)")}, "path")},
		{Name: "list_files", Description: "List files under a directory (default: the workspace root), one level deep unless recursive.",
			Parameters: obj(map[string]any{"path": str("directory, optional"), "recursive": map[string]any{"type": "boolean"}})},
		{Name: "search_files", Description: "Search file contents with a regular expression. Returns path:line: text, capped.",
			Parameters: obj(map[string]any{"pattern": str("Go/RE2 regular expression"), "path": str("directory or file to search, optional"), "glob": str("only files matching this glob, e.g. *.go (optional)")}, "pattern")},
		{Name: "edit_file", Description: "Replace one exact, unique occurrence of old with new in a file. Fails if old is missing or ambiguous; then read the file and try again with more context.",
			Parameters: obj(map[string]any{"path": str("file path"), "old": str("exact text to replace"), "new": str("replacement text")}, "path", "old", "new")},
		{Name: "write_file", Description: "Create or overwrite a whole file. Prefer edit_file for changes to existing files.",
			Parameters: obj(map[string]any{"path": str("file path"), "content": str("full file content")}, "path", "content")},
		{Name: "run", Description: "Run a shell command in the workspace (tests, builds, git, scripts). Output is distilled to the first and last lines. Two-minute limit.",
			Parameters: obj(map[string]any{"command": str("the command, as for sh -c")}, "command")},
		{Name: "open_path", Description: "Ask the person for a directory you are not allowed to reach yet. They see what you asked for and why, and answer. Use it when what you need is somewhere else on this machine.",
			Parameters: obj(map[string]any{"path": str("absolute path to the directory"), "why": str("one line: what you need it for")}, "path", "why")},
		{Name: "where", Description: "List the directories you are currently allowed to work in.",
			Parameters: obj(map[string]any{})},
	}
}

// Call runs one workspace tool. Errors come back as text for the model.
func (w *Workspace) Call(ctx context.Context, name string, args map[string]any) (string, bool) {
	s := func(k string) string {
		v, _ := args[k].(string)
		return v
	}
	n := func(k string) int {
		v, _ := args[k].(float64)
		return int(v)
	}
	switch name {
	case "read_file":
		abs, err := w.resolve(s("path"))
		if err != nil {
			return "error: " + err.Error(), true
		}
		raw, err := os.ReadFile(abs)
		if err != nil {
			return "error: " + err.Error(), true
		}
		if len(raw) > readMaxBytes {
			raw = raw[:readMaxBytes]
		}
		lines := strings.Split(string(raw), "\n")
		start, end := n("start"), n("end")
		if start < 1 {
			start = 1
		}
		if end < 1 || end > len(lines) {
			end = len(lines)
		}
		if start > end {
			return "error: empty range", true
		}
		var sb strings.Builder
		for i := start; i <= end; i++ {
			fmt.Fprintf(&sb, "%d\t%s\n", i, lines[i-1])
		}
		out := sb.String()
		if len(out) > 60_000 {
			out = out[:60_000] + "\n… (truncated; read a smaller range)"
		}
		return out, true
	case "list_files":
		p := s("path")
		if p == "" {
			p = "."
		}
		_ = p
		abs, err := w.resolve(p)
		if err != nil {
			return "error: " + err.Error(), true
		}
		rec, _ := args["recursive"].(bool)
		var out []string
		filepath.WalkDir(abs, func(q string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if q != abs && (skipDirs[d.Name()] || !rec) {
					if q != abs {
						out = append(out, w.rel(q)+"/")
					}
					return filepath.SkipDir
				}
				return nil
			}
			out = append(out, w.rel(q))
			if len(out) >= 500 {
				return filepath.SkipAll
			}
			return nil
		})
		if len(out) == 0 {
			return "(empty)", true
		}
		return strings.Join(out, "\n"), true
	case "search_files":
		re, err := regexp.Compile(s("pattern"))
		if err != nil {
			return "error: bad pattern: " + err.Error(), true
		}
		p := s("path")
		if p == "" {
			p = "."
		}
		abs, err := w.resolve(p)
		if err != nil {
			return "error: " + err.Error(), true
		}
		glob := s("glob")
		var sb strings.Builder
		hits := 0
		filepath.WalkDir(abs, func(q string, d os.DirEntry, err error) error {
			if err != nil || hits >= 200 {
				return nil
			}
			if d.IsDir() {
				if q != abs && skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if glob != "" {
				if ok, _ := filepath.Match(glob, d.Name()); !ok {
					return nil
				}
			}
			info, err := d.Info()
			if err != nil || info.Size() > 2_000_000 {
				return nil
			}
			raw, err := os.ReadFile(q)
			if err != nil || bytes.IndexByte(raw[:min(len(raw), 8000)], 0) >= 0 {
				return nil
			}
			for i, line := range strings.Split(string(raw), "\n") {
				if re.MatchString(line) {
					fmt.Fprintf(&sb, "%s:%d: %s\n", w.rel(q), i+1, trunc(strings.TrimSpace(line), 200))
					hits++
					if hits >= 200 {
						break
					}
				}
			}
			return nil
		})
		if hits == 0 {
			return "no matches", true
		}
		if hits >= 200 {
			sb.WriteString("… (capped at 200 matches; narrow the pattern or path)\n")
		}
		return sb.String(), true
	case "edit_file":
		abs, err := w.resolve(s("path"))
		if err != nil {
			return "error: " + err.Error(), true
		}
		raw, err := os.ReadFile(abs)
		if err != nil {
			return "error: " + err.Error(), true
		}
		old, nw := s("old"), s("new")
		if old == "" {
			return "error: old is required", true
		}
		switch strings.Count(string(raw), old) {
		case 0:
			return "error: old text not found; read the file and copy it exactly", true
		case 1:
		default:
			return "error: old text occurs more than once; include more surrounding context", true
		}
		out := strings.Replace(string(raw), old, nw, 1)
		if err := os.WriteFile(abs, []byte(out), 0o644); err != nil {
			return "error: " + err.Error(), true
		}
		return fmt.Sprintf("edited %s", w.rel(abs)), true
	case "write_file":
		abs, err := w.resolve(s("path"))
		if err != nil {
			return "error: " + err.Error(), true
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return "error: " + err.Error(), true
		}
		if err := os.WriteFile(abs, []byte(s("content")), 0o644); err != nil {
			return "error: " + err.Error(), true
		}
		return fmt.Sprintf("wrote %s (%d bytes)", w.rel(abs), len(s("content"))), true
	case "where":
		var sb strings.Builder
		for _, r := range w.roots() {
			sb.WriteString(r + "\n")
		}
		sb.WriteString("\nAnywhere else, ask with open_path.")
		return sb.String(), true
	case "open_path":
		abs, err := w.RequestPath(ctx, s("path"), s("why"))
		if err != nil {
			return "refused: " + err.Error(), true
		}
		return "you may now work in " + abs, true
	case "run":
		cmdline := strings.TrimSpace(s("command"))
		if cmdline == "" {
			return "error: command is required", true
		}
		cctx, cancel := context.WithTimeout(ctx, runTimeout)
		defer cancel()
		cmd := shellCommand(cctx, cmdline)
		cmd.Dir = w.Root
		cmd.Env = append(os.Environ(), "CI=1", "NO_COLOR=1", "TERM=dumb")
		var buf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &buf, &buf
		start := time.Now()
		err := cmd.Run()
		took := time.Since(start).Round(100 * time.Millisecond)
		status := "exit 0"
		if err != nil {
			if cctx.Err() == context.DeadlineExceeded {
				status = "timed out after 2m"
			} else if ee, ok := err.(*exec.ExitError); ok {
				status = fmt.Sprintf("exit %d", ee.ExitCode())
			} else {
				status = "error: " + err.Error()
			}
		}
		return fmt.Sprintf("$ %s\n[%s, %s]\n%s", cmdline, status, took, distill(buf.String())), true
	}
	return "", false
}

// shellCommand runs a command line through whatever shell this machine has.
func shellCommand(ctx context.Context, line string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/c", line)
	}
	return exec.CommandContext(ctx, "sh", "-c", line)
}

// distill keeps the head and tail of long output. The middle of a test log
// is rarely where the answer is; the first error and the summary are.
func distill(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(no output)"
	}
	lines := strings.Split(s, "\n")
	if len(lines) > runHeadLines+runTailLines {
		dropped := len(lines) - runHeadLines - runTailLines
		lines = append(append(lines[:runHeadLines], fmt.Sprintf("… (%d lines omitted)", dropped)), lines[len(lines)-runTailLines:]...)
	}
	out := strings.Join(lines, "\n")
	if len(out) > runMaxChars {
		out = out[:runMaxChars/2] + "\n… (truncated)\n" + out[len(out)-runMaxChars/2:]
	}
	return out
}
