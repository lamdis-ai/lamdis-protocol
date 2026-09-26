package api

import (
	"regexp"
	"strings"
	"testing"
)

// The sign-in page has to accept both code lengths Cognito sends.
//
// A sign-in challenge is 8 characters; a first-time confirmation is 6. The
// page assumed 6 and truncated, so an 8-character code was cut to its first
// six and submitted automatically — the person was holding the right code and
// being told it was wrong, with nothing they could do about it.
func TestSignInPageAcceptsBothCodeLengths(t *testing.T) {
	page := signInPageHTML

	if strings.Contains(page, "slice(0, 6)") {
		t.Error("the page still truncates codes to six characters")
	}
	if !strings.Contains(page, "slice(0, 8)") {
		t.Error("the page does not allow an eight-character code")
	}
	if m := regexp.MustCompile(`maxlength="(\d+)"`).FindStringSubmatch(page); m != nil {
		if m[1] != "8" {
			t.Errorf("the code field caps at %s characters", m[1])
		}
	} else {
		t.Error("no maxlength on the code field")
	}
	// It must not submit itself at six, or an eight-character code is sent
	// before the person has finished typing it.
	if strings.Contains(page, "length === 6) { verifyCode(); }") {
		t.Error("the page submits at six characters, cutting off longer codes")
	}
	if !strings.Contains(page, "length === 8) { verifyCode(); }") {
		t.Error("the page never submits a completed eight-character code on its own")
	}
	// Six is still valid and must remain manually submittable.
	if !strings.Contains(page, "cleaned.length < 6") {
		t.Error("a six-character code cannot be submitted")
	}
	// And the copy must not promise a length.
	if strings.Contains(page, "six-digit code") {
		t.Error("the page still tells people to expect a six-digit code")
	}
}

// A class that sets display silently defeats the hidden attribute, so an
// element marked hidden renders anyway. Every page shares one stylesheet, so
// the fix belongs there and must stay there.
func TestHiddenAttributeIsHonoured(t *testing.T) {
	if !strings.Contains(themeCSS, "[hidden] { display: none !important; }") {
		t.Error("the shared stylesheet does not force [hidden] to hide; " +
			"any component with a display rule will ignore it")
	}
}

// Every operator page must use the shared design system rather than its own
// copy of the palette, or the four surfaces drift apart.
func TestPagesShareOneDesignSystem(t *testing.T) {
	pages := map[string]string{
		"board":   boardPageHTML,
		"console": consolePageHTML,
		"work":    workPageHTML,
		"signin":  signInPageHTML,
	}
	for name, html := range pages {
		if !strings.Contains(html, "--gold:") {
			t.Errorf("%s does not include the shared theme", name)
		}
		// Two definitions of the palette means two palettes eventually.
		if n := strings.Count(html, "--gold:"); n != 1 {
			t.Errorf("%s defines the palette %d times", name, n)
		}
	}
}
