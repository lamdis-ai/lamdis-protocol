package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Higher-level code tools. The six primitives stay, but a model that has to
// compose "where is this defined" out of regular expressions spends turns
// doing what the harness can do in one. None of these tell the model how
// they work: a regex today could be an LSP tomorrow without the model ever
// knowing.

var sourceExt = map[string]bool{".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
	".py": true, ".rs": true, ".java": true, ".kt": true, ".swift": true, ".rb": true, ".php": true, ".cs": true, ".c": true,
	".h": true, ".cc": true, ".cpp": true, ".hpp": true, ".scala": true, ".dart": true, ".vue": true, ".svelte": true}

// symbolDef matches a top-level, likely-important definition, capturing its
// name. Deliberately crude: it is a map, not a compiler.
var symbolDef = regexp.MustCompile(`^(?:export\s+(?:default\s+)?(?:async\s+)?)?(?:pub(?:\(crate\))?\s+)?(?:public\s+|abstract\s+|final\s+|open\s+)*` +
	`(?:func\s+(?:\([^)]*\)\s*)?|type\s+|class\s+|interface\s+|struct\s+|enum\s+|trait\s+|def\s+|fn\s+|function\s+|protocol\s+)` +
	`([A-Za-z_][A-Za-z0-9_]*)`)

// RepoProfile is the cheap, stable description of a repository: what it is
// built with, how it is checked, and the names that matter. It goes at the
// start of the prompt with the file map, once per session.
func (w *Workspace) RepoProfile() string {
	if !worthMapping(w.Root) {
		return ""
	}
	root := resolveSymlinks(w.Root)
	var sb strings.Builder
	projects := projectsUnder(root)
	if len(projects) > 0 {
		sb.WriteString("Detected:\n")
		for i, p := range projects {
			if i >= 12 {
				fmt.Fprintf(&sb, "  … and %d more packages\n", len(projects)-i)
				break
			}
			where := w.rel(p.Dir)
			fmt.Fprintf(&sb, "  %s: %s", where, strings.Join(p.Stack, ", "))
			var cmds []string
			for _, c := range p.checks(nil, true) {
				cmds = append(cmds, c.Kind+"=`"+c.Command+"`")
			}
			if len(cmds) > 0 {
				sb.WriteString(" · checks " + strings.Join(cmds, " "))
			}
			sb.WriteString("\n")
		}
	}
	if syms := w.keySymbols(root, 160); syms != "" {
		sb.WriteString("Key definitions (file: names):\n" + syms)
	}
	return sb.String()
}

// keySymbols lists exported or top-level definitions per file, skipping
// tests, capped at max names in all.
func (w *Workspace) keySymbols(root string, max int) string {
	type fileSyms struct {
		path  string
		names []string
	}
	var files []fileSyms
	total, scanned := 0, 0
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || total >= max || scanned >= 600 {
			return nil
		}
		if d.IsDir() {
			if p != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		if !sourceExt[ext] || isTestFile(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 200_000 {
			return nil
		}
		scanned++
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		var names []string
		for _, line := range strings.Split(string(raw), "\n") {
			if len(line) == 0 || line[0] == ' ' || line[0] == '\t' {
				continue // top level only
			}
			m := symbolDef.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			name := m[1]
			// In Go only exported names are worth a line in the map.
			if ext == ".go" && (name[0] < 'A' || name[0] > 'Z') {
				continue
			}
			names = append(names, name)
			if len(names) >= 12 {
				break
			}
		}
		if len(names) > 0 {
			files = append(files, fileSyms{w.rel(p), names})
			total += len(names)
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	var sb strings.Builder
	for _, f := range files {
		fmt.Fprintf(&sb, "  %s: %s\n", f.path, strings.Join(f.names, ", "))
	}
	return sb.String()
}

// codeSpecs are the higher-level tools.
func codeSpecs() []ToolSpec {
	obj := func(props map[string]any, req ...string) map[string]any {
		m := map[string]any{"type": "object", "properties": props}
		if len(req) > 0 {
			m["required"] = req
		}
		return m
	}
	str := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	boolean := func(d string) map[string]any { return map[string]any{"type": "boolean", "description": d} }
	files := map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "file paths"}
	return []ToolSpec{
		{Name: "find_symbol", Description: "Find where a function, type, class, method or constant is defined. Accepts Name or Type.method. Returns path:line: definition.",
			Parameters: obj(map[string]any{"name": str("symbol name, e.g. PaymentService or AuthService.refresh")}, "name")},
		{Name: "find_references", Description: "Find every use of an identifier (whole-word match) across the workspace, excluding vendored and build directories.",
			Parameters: obj(map[string]any{"name": str("identifier"), "path": str("limit to this directory, optional")}, "name")},
		{Name: "find_tests", Description: "Find the test files that cover a source file, by naming convention and by which tests import it.",
			Parameters: obj(map[string]any{"path": str("source file path")}, "path")},
		{Name: "git_status", Description: "Show the branch and which files are modified, staged or untracked.", Parameters: obj(map[string]any{})},
		{Name: "git_diff", Description: "Show uncommitted changes as a unified diff, for one path or everything.",
			Parameters: obj(map[string]any{"path": str("file or directory, optional"), "staged": boolean("staged changes only")})},
		{Name: "run_tests", Description: "Run the project's own tests, using the detected test runner. With related_to, only the tests for those files where the runner supports it. Prefer this to guessing a test command.",
			Parameters: obj(map[string]any{"related_to": files, "all": boolean("run the whole suite")})},
		{Name: "run_checks", Description: "Run the detected build, typecheck, lint and test checks for the given changed files (or the whole project). Returns each check's status and the failing lines.",
			Parameters: obj(map[string]any{"files": files})},
	}
}

// codeReadOnly are the code tools that only look.
var codeReadOnly = map[string]bool{"find_symbol": true, "find_references": true, "find_tests": true, "git_status": true, "git_diff": true}

func (w *Workspace) callCode(ctx context.Context, name string, args map[string]any) (string, bool) {
	s := func(k string) string {
		v, _ := args[k].(string)
		return strings.TrimSpace(v)
	}
	list := func(k string) []string {
		raw, _ := args[k].([]any)
		var out []string
		for _, v := range raw {
			if p, ok := v.(string); ok && strings.TrimSpace(p) != "" {
				if abs, err := w.resolve(strings.TrimSpace(p)); err == nil {
					out = append(out, abs)
				}
			}
		}
		return out
	}
	root := resolveSymlinks(w.Root)
	switch name {
	case "find_symbol":
		sym := s("name")
		if sym == "" {
			return "error: name is required", true
		}
		owner, method, _ := strings.Cut(sym, ".")
		if method != "" {
			// Type.method: a Go receiver, or a method inside a class body.
			re := regexp.MustCompile(`(func\s*\([^)]*\b` + regexp.QuoteMeta(owner) + `\)\s*` + regexp.QuoteMeta(method) + `\b|^\s+(?:public\s+|private\s+|protected\s+|static\s+|async\s+|override\s+)*(?:def\s+|fn\s+|func\s+)?` + regexp.QuoteMeta(method) + `\s*[(<])`)
			hits := w.grep(root, re, 60)
			if len(hits) == 0 {
				return "no definition found for " + sym + "; try find_symbol " + method, true
			}
			return strings.Join(hits, "\n"), true
		}
		q := regexp.QuoteMeta(sym)
		re := regexp.MustCompile(`(?:func\s+(?:\([^)]*\)\s*)?|type\s+|class\s+|interface\s+|struct\s+|enum\s+|trait\s+|impl(?:<[^>]*>)?\s+|def\s+|fn\s+|function\s*\*?\s*|protocol\s+|(?:const|let|var|val)\s+)` + q + `\b|\b` + q + `\s*[:=]\s*(?:async\s*)?(?:function|\()`)
		hits := w.grep(root, re, 60)
		if len(hits) == 0 {
			return "no definition found for " + sym + "; find_references may show where it is used", true
		}
		return strings.Join(hits, "\n"), true
	case "find_references":
		sym := s("name")
		if sym == "" {
			return "error: name is required", true
		}
		dir := root
		if p := s("path"); p != "" {
			abs, err := w.resolve(p)
			if err != nil {
				return "error: " + err.Error(), true
			}
			dir = abs
		}
		hits := w.grep(dir, regexp.MustCompile(`\b`+regexp.QuoteMeta(sym)+`\b`), 200)
		if len(hits) == 0 {
			return "no references", true
		}
		out := strings.Join(hits, "\n")
		if len(hits) >= 200 {
			out += "\n… (capped at 200; narrow with path)"
		}
		return out, true
	case "find_tests":
		abs, err := w.resolve(s("path"))
		if err != nil {
			return "error: " + err.Error(), true
		}
		found := map[string]bool{}
		for _, t := range testsCovering(root, abs, 40) {
			found[w.rel(t)] = true
		}
		if len(found) == 0 {
			return "no tests found for " + w.rel(abs), true
		}
		return strings.Join(sortedKeys(found), "\n"), true
	case "git_status":
		return gitOut(ctx, root, "status", "--short", "--branch"), true
	case "git_diff":
		gargs := []string{"diff", "--no-color", "--stat", "--patch"}
		if b, _ := args["staged"].(bool); b {
			gargs = append(gargs, "--cached")
		}
		if p := s("path"); p != "" {
			abs, err := w.resolve(p)
			if err != nil {
				return "error: " + err.Error(), true
			}
			gargs = append(gargs, "--", abs)
		}
		out := gitOut(ctx, root, gargs...)
		if len(out) > 40_000 {
			out = out[:40_000] + "\n… (truncated; diff a single path)"
		}
		return out, true
	case "run_tests":
		files := list("related_to")
		all, _ := args["all"].(bool)
		var tests []check
		for _, c := range planChecks(root, files, all) {
			if c.Kind == "test" {
				tests = append(tests, c)
			}
		}
		if len(tests) == 0 {
			return "no test runner detected here; use run with the project's command", true
		}
		var rs []checkResult
		for _, c := range tests {
			rs = append(rs, runCheck(ctx, c))
		}
		return describeChecks(rs) + tailOf(rs), true
	case "run_checks":
		cs := planChecks(root, list("files"), false)
		if len(cs) == 0 {
			return "no checks detected here; use run with the project's command", true
		}
		var rs []checkResult
		for _, c := range cs {
			rs = append(rs, runCheck(ctx, c))
		}
		return describeChecks(rs) + tailOf(rs), true
	}
	return "", false
}

// tailOf adds the distilled output of the first failing check, since the
// failure lines alone sometimes miss the context a fix needs.
func tailOf(rs []checkResult) string {
	for _, r := range rs {
		if r.Status == "failed" || r.Status == "timed_out" || r.Status == statusFailingBefore {
			return "\nOutput of " + r.Command + ":\n" + trunc(r.Output, 6000)
		}
	}
	return ""
}

// grep walks dir for lines matching re in source and text files.
func (w *Workspace) grep(dir string, re *regexp.Regexp, max int) []string {
	var hits []string
	w.walkText(dir, func(path string, raw []byte) bool {
		for i, line := range strings.Split(string(raw), "\n") {
			if re.MatchString(line) {
				hits = append(hits, fmt.Sprintf("%s:%d: %s", w.rel(path), i+1, trunc(strings.TrimSpace(line), 200)))
				if len(hits) >= max {
					return false
				}
			}
		}
		return true
	})
	return hits
}

// grepFiles returns the files (filtered by name) that contain a match.
func (w *Workspace) grepFiles(dir string, re *regexp.Regexp, keep func(string) bool, max int) []string {
	var out []string
	w.walkText(dir, func(path string, raw []byte) bool {
		if keep(filepath.Base(path)) && re.Match(raw) {
			out = append(out, w.rel(path))
		}
		return len(out) < max
	})
	return out
}

func (w *Workspace) walkText(dir string, fn func(path string, raw []byte) bool) {
	n := 0
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != dir && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if n++; n > 30000 {
			return filepath.SkipAll
		}
		info, err := d.Info()
		if err != nil || info.Size() > 1_000_000 {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil || bytes.IndexByte(raw[:min(len(raw), 8000)], 0) >= 0 {
			return nil
		}
		if !fn(p, raw) {
			return filepath.SkipAll
		}
		return nil
	})
}

// gitOut runs git and returns its output, or a plain sentence when this is
// not a repository.
func gitOut(ctx context.Context, dir string, args ...string) string {
	out, err := gitRaw(ctx, dir, args...)
	if err != nil {
		if strings.Contains(out, "not a git repository") {
			return "not a git repository"
		}
		return strings.TrimSpace("git " + args[0] + " failed: " + out)
	}
	if strings.TrimSpace(out) == "" {
		return "(no changes)"
	}
	return out
}

func gitRaw(ctx context.Context, dir string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_PAGER=cat", "GIT_TERMINAL_PROMPT=0")
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	return buf.String(), err
}
