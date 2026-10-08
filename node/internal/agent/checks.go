package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// The harness, not the model, knows how a project is checked. A model asked
// to "run the tests" spends turns rediscovering that this is a pnpm
// workspace with vitest, and then runs the whole suite. Detecting it once,
// from the files that say so, makes "verify what you changed" a single step
// the controller can take on its own.

// project is one buildable unit: the nearest directory with a manifest.
type codeProject struct {
	Dir   string // absolute
	Kind  string // go, node, rust, python, make
	PM    string // node: npm, pnpm, yarn, bun
	Stack []string
	// scripts are package.json scripts, for node.
	scripts map[string]string
	deps    map[string]bool
}

// check is one command the harness runs to verify a change.
type check struct {
	Kind    string `json:"kind"` // build, typecheck, lint, test
	Dir     string `json:"dir"`
	Command string `json:"command"`
}

// checkResult is what a check printed, distilled to what a repair needs.
type checkResult struct {
	check
	Status   string   `json:"status"` // passed, failed, timed_out, unavailable
	Exit     int      `json:"exit"`
	Took     string   `json:"took"`
	Failures []string `json:"failures,omitempty"`
	Output   string   `json:"-"`
}

var manifests = []string{"go.mod", "package.json", "Cargo.toml", "pyproject.toml", "setup.py", "setup.cfg", "pytest.ini", "Makefile"}

// detectProject reads the manifest in dir, or returns nil when there is none.
// A directory with more than one manifest answers with the first of them.
func detectProject(dir string) *codeProject {
	if ps := detectProjects(dir); len(ps) > 0 {
		return ps[0]
	}
	return nil
}

// detectProjects reads every manifest in dir. One directory is often more
// than one project: a Python package with a package.json for its front-end
// assets, a Go module with a Makefile. Which one verifies a change depends
// on the files changed, so none of them may hide the others.
func detectProjects(dir string) []*codeProject {
	var out []*codeProject
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}
	if has("go.mod") {
		out = append(out, &codeProject{Dir: dir, Kind: "go", Stack: []string{"Go"}})
	}
	if has("package.json") {
		p := &codeProject{Dir: dir, Kind: "node", PM: "npm", scripts: map[string]string{}, deps: map[string]bool{}}
		var pkg struct {
			Scripts         map[string]string `json:"scripts"`
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
			Workspaces      json.RawMessage   `json:"workspaces"`
		}
		if raw, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
			json.Unmarshal(raw, &pkg)
		}
		if pkg.Scripts != nil {
			p.scripts = pkg.Scripts
		}
		for k := range pkg.Dependencies {
			p.deps[k] = true
		}
		for k := range pkg.DevDependencies {
			p.deps[k] = true
		}
		for _, lock := range []struct{ file, pm string }{{"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}, {"bun.lockb", "bun"}, {"bun.lock", "bun"}} {
			if has(lock.file) || hasUp(dir, lock.file) {
				p.PM = lock.pm
				break
			}
		}
		p.Stack = []string{"JavaScript"}
		if has("tsconfig.json") || p.deps["typescript"] {
			p.Stack[0] = "TypeScript"
		}
		for _, d := range []struct{ dep, name string }{{"next", "Next.js"}, {"react", "React"}, {"vue", "Vue"}, {"svelte", "Svelte"},
			{"express", "Express"}, {"fastify", "Fastify"}, {"@nestjs/core", "NestJS"}, {"prisma", "Prisma"}, {"vite", "Vite"},
			{"vitest", "Vitest"}, {"jest", "Jest"}, {"mocha", "Mocha"}, {"@playwright/test", "Playwright"}, {"eslint", "ESLint"}} {
			if p.deps[d.dep] {
				p.Stack = append(p.Stack, d.name)
			}
		}
		if len(pkg.Workspaces) > 0 || has("pnpm-workspace.yaml") {
			p.Stack = append(p.Stack, p.PM+" workspace")
		}
		if has("turbo.json") {
			p.Stack = append(p.Stack, "Turborepo")
		}
		out = append(out, p)
	}
	if has("Cargo.toml") {
		out = append(out, &codeProject{Dir: dir, Kind: "rust", Stack: []string{"Rust"}})
	}
	if has("pyproject.toml") || has("setup.py") || has("setup.cfg") || has("pytest.ini") {
		p := &codeProject{Dir: dir, Kind: "python", Stack: []string{"Python"}}
		raw, _ := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
		for _, d := range []struct{ marker, name string }{{"pytest", "pytest"}, {"django", "Django"}, {"fastapi", "FastAPI"}, {"flask", "Flask"}, {"ruff", "Ruff"}, {"mypy", "mypy"}} {
			if bytes.Contains(bytes.ToLower(raw), []byte(d.marker)) {
				p.Stack = append(p.Stack, d.name)
			}
		}
		out = append(out, p)
	}
	if has("Makefile") {
		out = append(out, &codeProject{Dir: dir, Kind: "make", Stack: []string{"Make"}})
	}
	return out
}

// hasUp looks for a file in the parents of dir, for lockfiles that live at a
// monorepo root above the package being changed. It stops at a .git.
func hasUp(dir, name string) bool {
	for d := filepath.Dir(dir); d != filepath.Dir(d); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, name)); err == nil {
			return true
		}
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return false
		}
	}
	return false
}

// projectFor finds the project a file belongs to: the nearest manifest at or
// above it, never above root, for the file's own language. A .py file next to
// a package.json belongs to the Python project above it, not to npm. When no
// manifest speaks the file's language, the nearest one still answers.
func projectFor(root, file string) *codeProject {
	dir := file
	if st, err := os.Stat(file); err != nil || !st.IsDir() {
		dir = filepath.Dir(file)
	}
	var nearest *codeProject
	for {
		for _, p := range detectProjects(dir) {
			if nearest == nil {
				nearest = p
			}
			if p.owns(file) {
				return p
			}
		}
		if dir == root || !strings.HasPrefix(dir, root) || dir == filepath.Dir(dir) {
			return nearest
		}
		dir = filepath.Dir(dir)
	}
}

// owns reports whether a file is written in this project's language. A
// Makefile drives whatever it is pointed at, so it owns nothing in
// particular and only ever answers as the nearest manifest.
func (p *codeProject) owns(file string) bool {
	switch p.Kind {
	case "go":
		return strings.HasSuffix(file, ".go")
	case "node":
		return jsSource.MatchString(file)
	case "rust":
		return strings.HasSuffix(file, ".rs")
	case "python":
		return strings.HasSuffix(file, ".py") || strings.HasSuffix(file, ".pyi")
	}
	return false
}

// projectsUnder lists every project at the root and one or two levels down,
// for the repository profile. Monorepos keep packages at apps/x, packages/y.
func projectsUnder(root string) []*codeProject {
	var out []*codeProject
	seen := map[string]bool{}
	add := func(dir string) {
		if seen[dir] {
			return
		}
		seen[dir] = true
		out = append(out, detectProjects(dir)...)
	}
	add(root)
	for depth, dirs := 0, []string{root}; depth < 2; depth++ {
		var next []string
		for _, d := range dirs {
			entries, _ := os.ReadDir(d)
			for _, e := range entries {
				if !e.IsDir() || skipDirs[e.Name()] || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				sub := filepath.Join(d, e.Name())
				add(sub)
				next = append(next, sub)
			}
		}
		if len(next) > 60 {
			next = next[:60]
		}
		dirs = next
	}
	return out
}

func (p *codeProject) run(script string) string {
	switch p.PM {
	case "npm":
		if script == "test" {
			return "npm test --silent"
		}
		return "npm run --silent " + script
	case "yarn":
		return "yarn -s " + script
	default:
		return p.PM + " run " + script
	}
}

func (p *codeProject) exec(bin string) string {
	switch p.PM {
	case "pnpm":
		return "pnpm exec " + bin
	case "yarn":
		return "yarn " + bin
	case "bun":
		return "bunx " + bin
	}
	return "npx --no-install " + bin
}

func (p *codeProject) script(names ...string) string {
	for _, n := range names {
		if s, ok := p.scripts[n]; ok && !strings.Contains(s, "no test specified") {
			return n
		}
	}
	return ""
}

// checks is what verifies a change to files in this project. scoped narrows
// tests to what the change touches when the tooling can; full runs all of it.
func (p *codeProject) checks(files []string, full bool) []check {
	rel := func(f string) string {
		r, err := filepath.Rel(p.Dir, f)
		if err != nil {
			return f
		}
		return r
	}
	mk := func(kind, cmd string) check { return check{Kind: kind, Dir: p.Dir, Command: cmd} }
	var out []check
	switch p.Kind {
	case "go":
		pkgs := map[string]bool{}
		for _, f := range files {
			if strings.HasSuffix(f, ".go") {
				d := rel(filepath.Dir(f))
				if d == "." {
					pkgs["."] = true
				} else {
					pkgs["./"+filepath.ToSlash(d)] = true
				}
			}
		}
		target := "./..."
		if !full && len(pkgs) > 0 {
			target = strings.Join(sortedKeys(pkgs), " ")
		}
		out = append(out, mk("build", "go build ./..."), mk("lint", "go vet "+target), mk("test", "go test "+target))
	case "node":
		if s := p.script("typecheck", "type-check", "tsc", "check-types"); s != "" {
			out = append(out, mk("typecheck", p.run(s)))
		} else if _, err := os.Stat(filepath.Join(p.Dir, "tsconfig.json")); err == nil && p.deps["typescript"] {
			out = append(out, mk("typecheck", p.exec("tsc --noEmit")))
		}
		if s := p.script("lint"); s != "" {
			out = append(out, mk("lint", p.run(s)))
		}
		var src []string
		for _, f := range files {
			if jsSource.MatchString(f) {
				src = append(src, rel(f))
			}
		}
		switch {
		case !full && len(src) > 0 && p.deps["vitest"]:
			out = append(out, mk("test", p.exec("vitest related --run "+shellJoin(src))))
		case !full && len(src) > 0 && p.deps["jest"]:
			out = append(out, mk("test", p.exec("jest --findRelatedTests --passWithNoTests "+shellJoin(src))))
		default:
			if s := p.script("test"); s != "" {
				out = append(out, mk("test", p.run(s)))
			}
		}
		if len(out) == 0 {
			if s := p.script("build"); s != "" {
				out = append(out, mk("build", p.run(s)))
			}
		}
	case "rust":
		out = append(out, mk("build", "cargo check --quiet"), mk("test", "cargo test --quiet"))
	case "python":
		var py []string
		for _, f := range files {
			if strings.HasSuffix(f, ".py") {
				if _, err := os.Stat(f); err == nil {
					py = append(py, rel(f))
				}
			}
		}
		if len(py) > 0 {
			out = append(out, mk("build", "python3 -m py_compile "+shellJoin(py)))
		}
		tests := []string{}
		if !full {
			for _, f := range files {
				for _, t := range testsCovering(p.Dir, f, maxScopedTests) {
					tests = append(tests, rel(t))
				}
			}
			if len(tests) > maxScopedTests {
				tests = dedupe(tests)[:maxScopedTests]
			}
		}
		if len(tests) > 0 {
			out = append(out, mk("test", "python3 -m pytest -q "+shellJoin(dedupe(tests))))
		} else if hasPytest(p.Dir) {
			out = append(out, mk("test", "python3 -m pytest -q"))
		}
	case "make":
		raw, _ := os.ReadFile(filepath.Join(p.Dir, "Makefile"))
		if regexp.MustCompile(`(?m)^test:`).Match(raw) {
			out = append(out, mk("test", "make test"))
		} else if regexp.MustCompile(`(?m)^check:`).Match(raw) {
			out = append(out, mk("test", "make check"))
		}
	}
	return out
}

func hasPytest(dir string) bool {
	for _, n := range []string{"pytest.ini", "conftest.py", "tests", "test"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			return true
		}
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	return bytes.Contains(raw, []byte("pytest"))
}

// planChecks groups changed files by project and returns each project's
// checks. Files outside any project contribute nothing.
func planChecks(root string, files []string, full bool) []check {
	byDir := map[string]*codeProject{}
	grouped := map[string][]string{}
	for _, f := range files {
		p := projectFor(root, f)
		if p == nil {
			continue
		}
		byDir[p.Dir] = p
		grouped[p.Dir] = append(grouped[p.Dir], f)
	}
	if len(files) == 0 {
		if p := projectFor(root, root); p != nil {
			byDir[p.Dir], grouped[p.Dir] = p, nil
		}
	}
	var out []check
	for _, dir := range sortedKeys(grouped) {
		out = append(out, byDir[dir].checks(grouped[dir], full || len(grouped[dir]) == 0)...)
	}
	return out
}

const checkTimeout = 5 * time.Minute

var (
	jsSource  = regexp.MustCompile(`\.(m|c)?(t|j)sx?$`)
	shellSafe = regexp.MustCompile(`^[A-Za-z0-9_./@:+-]+$`)
)

var failureLine = regexp.MustCompile(`(?i)(^--- FAIL|^FAIL|\bFAIL\b|panic:|error[:\[ ]|\berror TS\d+|AssertionError|expected .* (got|received)|✗|×|^\s*at .+:\d+:\d+|:\d+:\d+: )`)

// runCheck runs one check and keeps what a repair would need: the status
// and the lines that name the failure, not four hundred lines of log.
func runCheck(ctx context.Context, c check) checkResult {
	cctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	cmd := shellCommand(cctx, c.Command)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), "CI=1", "NO_COLOR=1", "FORCE_COLOR=0", "TERM=dumb")
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	start := time.Now()
	err := cmd.Run()
	res := checkResult{check: c, Status: "passed", Took: time.Since(start).Round(100 * time.Millisecond).String(), Output: distill(buf.String())}
	if err != nil {
		res.Status, res.Exit = "failed", -1
		if ee, ok := err.(*exec.ExitError); ok {
			res.Exit = ee.ExitCode()
		}
		switch {
		case cctx.Err() == context.DeadlineExceeded:
			res.Status = "timed_out"
		case res.Exit == 127 || strings.Contains(buf.String(), "command not found") || strings.Contains(buf.String(), "not installed"):
			// A tool that is not installed is not a failure of the change.
			res.Status = "unavailable"
		}
		res.Failures = failureLines(buf.String(), 20)
	}
	return res
}

func failureLines(out string, max int) []string {
	var lines []string
	seen := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimRight(l, " \t\r")
		if l == "" || seen[l] || !failureLine.MatchString(l) {
			continue
		}
		seen[l] = true
		lines = append(lines, trunc(strings.TrimSpace(l), 240))
		if len(lines) >= max {
			break
		}
	}
	if len(lines) == 0 {
		// Nothing matched: the tail is where the summary usually is.
		all := strings.Split(strings.TrimSpace(out), "\n")
		if len(all) > 8 {
			all = all[len(all)-8:]
		}
		for _, l := range all {
			lines = append(lines, trunc(strings.TrimSpace(l), 240))
		}
	}
	return lines
}

// relatedTests finds test files for a source file by the naming conventions
// most ecosystems use. It does not open files; find_tests adds imports.
func relatedTests(root, file string) []string {
	base := filepath.Base(file)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if isTestFile(base) {
		return []string{file}
	}
	var want []string
	switch ext {
	case ".go":
		dir := filepath.Dir(file)
		want = append(want, filepath.Join(dir, stem+"_test.go"))
	case ".py":
		want = append(want, "test_"+stem+".py", stem+"_test.py")
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		for _, k := range []string{".test", ".spec"} {
			for _, e := range []string{".ts", ".tsx", ".js", ".jsx"} {
				want = append(want, stem+k+e)
			}
		}
	case ".rs", ".java", ".kt", ".swift", ".rb":
		want = append(want, stem+"Test"+ext, stem+"Tests"+ext, stem+"_test"+ext, stem+"_spec"+ext, "test_"+stem+ext)
	default:
		return nil
	}
	var out []string
	if ext == ".go" {
		if _, err := os.Stat(want[0]); err == nil {
			out = append(out, want[0])
		}
		return out
	}
	names := map[string]bool{}
	for _, w := range want {
		names[w] = true
	}
	n := 0
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if n++; n > 20000 {
			return filepath.SkipAll
		}
		if names[d.Name()] {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// maxScopedTests bounds how many test files a scoped check runs, so a
// change to a module half the project imports does not become the full suite.
const maxScopedTests = 8

// testsCovering is the tests that exercise a source file: the ones named
// after it first, then the ones that import it. A test that imports the
// changed module is the nearest evidence the change broke something, and
// it is often not named after the file at all.
func testsCovering(root, file string, max int) []string {
	out := relatedTests(root, file)
	seen := map[string]bool{}
	for _, t := range out {
		seen[t] = true
	}
	for _, t := range importingTests(root, file, max) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// importingTests finds test files, in the same language, that import the
// file's module: by its dotted path for Python, by its stem otherwise. Only
// test files are opened.
func importingTests(root, file string, max int) []string {
	base := filepath.Base(file)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if isTestFile(base) || len(stem) < 3 || stem == "index" || stem == "main" || stem == "__init__" || stem == "mod" {
		return nil
	}
	var re *regexp.Regexp
	sameLang := func(name string) bool { return filepath.Ext(name) == ext }
	switch {
	case ext == ".py":
		// pkg/sub/mod.py is imported as pkg.sub.mod, or mod from pkg.sub.
		rel, err := filepath.Rel(root, strings.TrimSuffix(file, ext))
		if err != nil {
			return nil
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		for i := range parts {
			if parts[i] == "src" || parts[i] == "lib" {
				continue
			}
			parts = parts[i:]
			break
		}
		dotted := regexp.QuoteMeta(strings.Join(parts, "."))
		parent := regexp.QuoteMeta(strings.Join(parts[:len(parts)-1], "."))
		pat := `(?m)^\s*(import\s+` + dotted + `\b|from\s+` + dotted + `\s+import\b`
		if parent != "" {
			pat += `|from\s+` + parent + `\s+import\s+[^\n]*\b` + regexp.QuoteMeta(stem) + `\b`
		}
		re = regexp.MustCompile(pat + `)`)
	case jsSource.MatchString(base):
		sameLang = func(name string) bool { return jsSource.MatchString(name) }
		re = regexp.MustCompile(`(import|require|from)\b[^\n]*['"/]` + regexp.QuoteMeta(stem) + `(\.[a-z]+)?['"]`)
	case ext == ".go":
		return nil // a Go package's tests sit beside it, named after it
	default:
		re = regexp.MustCompile(`(import|require|from|use)\b.*\b` + regexp.QuoteMeta(stem) + `\b`)
	}
	var out []string
	n := 0
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if n++; n > 30000 {
			return filepath.SkipAll
		}
		if !isTestFile(d.Name()) || !sameLang(d.Name()) || p == file {
			return nil
		}
		if raw, err := os.ReadFile(p); err == nil && len(raw) < 1_000_000 && re.Match(raw) {
			out = append(out, p)
			if len(out) >= max {
				return filepath.SkipAll
			}
		}
		return nil
	})
	return out
}

func isTestFile(name string) bool {
	return strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, "test_") || strings.Contains(name, ".test.") ||
		strings.Contains(name, ".spec.") || strings.HasSuffix(strings.TrimSuffix(name, filepath.Ext(name)), "_test")
}

func shellJoin(xs []string) string {
	out := make([]string, len(xs))
	for i, x := range xs {
		if shellSafe.MatchString(x) {
			out[i] = x
		} else {
			out[i] = "'" + strings.ReplaceAll(x, "'", `'\''`) + "'"
		}
	}
	return strings.Join(out, " ")
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// describeChecks is one line per check result, for the model and the record.
func describeChecks(rs []checkResult) string {
	var sb strings.Builder
	for _, r := range rs {
		fmt.Fprintf(&sb, "- [%s] %s (in %s): %s, exit %d, %s\n", r.Kind, r.Command, filepath.Base(r.Dir), r.Status, r.Exit, r.Took)
		for _, f := range r.Failures {
			sb.WriteString("    " + f + "\n")
		}
	}
	return sb.String()
}
