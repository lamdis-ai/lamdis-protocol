package agent

// How much of the machine the agent may work in.
//
// Asking about every directory is its own kind of broken: a question you
// answer twenty times is a question you stop reading. So the reach is a
// setting, chosen once, and the asking is only for what falls outside it.
//
// Three levels, and they mean what they say. The guard on credential stores
// is separate and on by default at every level, because "the agent may work
// in my home directory" and "the agent may read my ssh keys" are different
// sentences, and somebody saying the first rarely means the second.

import (
	"os"
	"path/filepath"
	"strings"
)

// Trust levels.
const (
	TrustProject = "project" // where you started it, and below
	TrustHome    = "home"    // anything under your home directory
	TrustAll     = "all"     // the whole machine
)

// TrustSays is one line describing a level, for a person about to pick one.
func TrustSays(level string) string {
	switch level {
	case TrustHome:
		return "anywhere under your home directory"
	case TrustAll:
		return "anywhere on this machine"
	default:
		return "where you started it, and below"
	}
}

// guarded are the places an agent has no business in unless somebody has
// deliberately said otherwise: keys, credentials, saved sessions, and the
// browser profiles that hold everything you are signed in to.
var guarded = []string{
	".ssh", ".gnupg", ".aws", ".azure", ".config/gcloud", ".kube", ".docker",
	".netrc", ".pgpass", ".npmrc", ".pypirc", ".gem/credentials",
	"Library/Keychains", "Library/Application Support/Google/Chrome",
	"Library/Application Support/Firefox", "Library/Application Support/BraveSoftware",
	".mozilla", ".config/google-chrome", ".config/chromium",
	".password-store", ".1password", ".local/share/keyrings",
	".lamdis", ".claude", ".codex",
}

// Guarded reports whether a path is somewhere the guard refuses, and names
// it, so the refusal can say which thing it is protecting.
func Guarded(abs string) (bool, string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, ""
	}
	clean := filepath.Clean(abs)
	for _, g := range guarded {
		p := filepath.Clean(filepath.Join(home, g))
		if clean == p || strings.HasPrefix(clean, p+string(filepath.Separator)) {
			return true, g
		}
	}
	// A file that is nothing but a secret, wherever it lives.
	switch filepath.Base(clean) {
	case ".env", "id_rsa", "id_ed25519", "credentials", ".npmrc", ".netrc":
		return true, filepath.Base(clean)
	}
	return false, ""
}

// Reach decides what a workspace may touch, from the settings and the place
// it was launched.
func Reach(cfg Config, root string) (roots []string, ask bool) {
	for _, p := range cfg.AllowPaths {
		if abs, err := filepath.Abs(p); err == nil {
			roots = append(roots, abs)
		}
	}
	switch cfg.Trust {
	case TrustAll:
		sep := string(filepath.Separator)
		if vol := filepath.VolumeName(root); vol != "" {
			sep = vol + sep
		}
		return append(roots, sep), false
	case TrustHome:
		if home, err := os.UserHomeDir(); err == nil {
			return append(roots, home), true
		}
	}
	return roots, true
}
