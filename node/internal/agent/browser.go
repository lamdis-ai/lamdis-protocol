package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// A browser the agent drives for the person: open a page, read it, click,
// type, and carry on through sites that have no API. Every account gets its
// own browser context (its own cookies, nothing shared), kept for a while
// between runs so a task can pause for the person and resume where it was.
//
// When a site needs a login, the agent hands the browser to the person: they
// see the live page and sign in themselves, so their password goes to the
// site and never through the agent. What the site leaves behind (cookies) is
// sealed with the account's vault key and restored next time.
//
// The browser runs wherever LAMDIS_BROWSER_CDP points: a Chrome beside the
// server, or a pool of them. On a laptop, the local Chrome.
//
// Nothing the browser loads may reach a private address: every request is
// paused and its host vetted, the same rule fetch_url follows.

// browserAllowPrivate lets tests point the browser at a local server.
var browserAllowPrivate = false

// BrowserPool hands out one browser session per account.
type BrowserPool struct {
	// CDP is a Chrome DevTools endpoint (http://host:9222 or ws://…). Empty
	// launches a local headless Chrome, if there is one.
	CDP string
	// Idle is how long an unused session lives. Default ten minutes.
	Idle time.Duration

	mu       sync.Mutex
	alloc    context.Context
	stop     context.CancelFunc
	browser  context.Context // the connected browser; accounts get contexts inside it
	sessions map[string]*BrowserSession
}

// BrowserSession is one account's browser.
type BrowserSession struct {
	pool    *BrowserPool
	key     string // the account's data directory
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	last    time.Time
	hostsOK sync.Map // host -> bool, vetted once per session
	// pinned keeps the session alive while the person has a step to do in
	// it (a hand-off or a confirmation), however long they take.
	pinned time.Time
	// refused counts the site's own requests that failed since the last
	// read: a site that blocks automated browsers looks like this.
	refused int
	fmu     sync.Mutex
}

// NewBrowserPool returns a pool, or nil when there is nowhere to run a browser.
func NewBrowserPool(cdp string) *BrowserPool {
	cdp = strings.TrimSpace(cdp)
	if cdp == "" && localChrome() == "" {
		return nil
	}
	return &BrowserPool{CDP: cdp, sessions: map[string]*BrowserSession{}}
}

func localChrome() string {
	for _, p := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/usr/bin/google-chrome", "/usr/bin/chromium", "/usr/bin/chromium-browser",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// root connects to (or starts) the browser once; every account's isolated
// context is made inside it.
func (p *BrowserPool) root() (context.Context, error) {
	if p.browser != nil && p.browser.Err() == nil {
		return p.browser, nil
	}
	if p.CDP != "" {
		p.alloc, p.stop = chromedp.NewRemoteAllocator(context.Background(), p.CDP)
	} else {
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.ExecPath(localChrome()), chromedp.WindowSize(1280, 800))
		p.alloc, p.stop = chromedp.NewExecAllocator(context.Background(), opts...)
	}
	b, _ := chromedp.NewContext(p.alloc)
	// The first Run creates the browser and belongs to b itself: a derived
	// context with a deadline would close the browser when it ended.
	if err := chromedp.Run(b); err != nil {
		p.stop()
		return nil, err
	}
	p.browser = b
	return b, nil
}

// Session returns the account's browser, starting one if needed.
func (p *BrowserPool) Session(key string) (*BrowserSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reap()
	if s := p.sessions[key]; s != nil && s.ctx.Err() == nil {
		s.last = time.Now()
		return s, nil
	}
	root, err := p.root()
	if err != nil {
		return nil, fmt.Errorf("could not reach the browser: %v", err)
	}
	// Each account gets its own browser context (its own cookies and
	// storage), with its tab in a new window: current Chrome will not open a
	// tab in a fresh context otherwise.
	var bcID cdp.BrowserContextID
	var tid target.ID
	err = chromedp.Run(root, chromedp.ActionFunc(func(c context.Context) error {
		bc := cdp.WithExecutor(c, chromedp.FromContext(c).Browser)
		var err error
		if bcID, err = target.CreateBrowserContext().WithDisposeOnDetach(true).Do(bc); err != nil {
			return err
		}
		tid, err = target.CreateTarget("about:blank").WithBrowserContextID(bcID).WithNewWindow(true).Do(bc)
		return err
	}))
	if err != nil {
		return nil, fmt.Errorf("could not open a browser for this account: %v", err)
	}
	ctx, closeTab := chromedp.NewContext(root, chromedp.WithTargetID(tid))
	cancel := func() {
		closeTab()
		chromedp.Run(root, chromedp.ActionFunc(func(c context.Context) error {
			return target.DisposeBrowserContext(bcID).Do(cdp.WithExecutor(c, chromedp.FromContext(c).Browser))
		}))
	}
	s := &BrowserSession{pool: p, key: key, ctx: ctx, cancel: cancel, last: time.Now()}
	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *fetch.EventRequestPaused:
			go s.vetRequest(e)
		case *network.EventResponseReceived:
			if (e.Type == network.ResourceTypeXHR || e.Type == network.ResourceTypeFetch) && (e.Response.Status == 403 || e.Response.Status == 429) {
				s.fmu.Lock()
				s.refused++
				s.fmu.Unlock()
			}
		case *network.EventLoadingFailed:
			if (e.Type == network.ResourceTypeXHR || e.Type == network.ResourceTypeFetch) && !e.Canceled && (e.CorsErrorStatus != nil || e.BlockedReason != "") {
				s.fmu.Lock()
				s.refused++
				s.fmu.Unlock()
			}
		}
	})
	// As above: the first Run makes this account's tab, so it runs on ctx.
	err = chromedp.Run(ctx,
		fetch.Enable(),
		network.Enable(),
		chromedp.EmulateViewport(1280, 800),
		chromedp.ActionFunc(func(c context.Context) error { return s.restoreCookies(c) }),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("could not start a browser: %v", err)
	}
	p.sessions[key] = s
	// A session that ended (idle, or a restart) picks up where it was.
	if u := s.lastURL(); u != "" {
		go func() {
			if _, err := browsableURL(u); err == nil {
				s.run(45*time.Second, chromedp.Navigate(u), settle())
			}
		}()
	}
	return s, nil
}

// Pin keeps the session open while the person has something to do in it.
func (s *BrowserSession) Pin(d time.Duration) {
	s.mu.Lock()
	s.pinned = time.Now().Add(d)
	s.mu.Unlock()
}

// Existing returns the account's browser only if one is open.
func (p *BrowserPool) Existing(key string) *BrowserSession {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s := p.sessions[key]; s != nil && s.ctx.Err() == nil {
		return s
	}
	return nil
}

func (p *BrowserPool) reap() {
	idle := p.Idle
	if idle == 0 {
		idle = 30 * time.Minute
	}
	for k, s := range p.sessions {
		if s.ctx.Err() == nil && time.Now().Before(s.pinned) {
			continue
		}
		if s.ctx.Err() != nil || time.Since(s.last) > idle {
			s.saveCookies()
			s.cancel()
			delete(p.sessions, k)
		}
	}
}

// vetRequest lets a request through only if its host is public.
func (s *BrowserSession) vetRequest(e *fetch.EventRequestPaused) {
	c := chromedp.FromContext(s.ctx)
	if c == nil || c.Target == nil {
		return
	}
	ctx := cdp.WithExecutor(s.ctx, c.Target)
	if s.allowedURL(e.Request.URL) {
		fetch.ContinueRequest(e.RequestID).Do(ctx)
	} else {
		fetch.FailRequest(e.RequestID, network.ErrorReasonBlockedByClient).Do(ctx)
	}
}

func (s *BrowserSession) allowedURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "data", "blob", "about":
		return true
	case "http", "https":
	default:
		return false
	}
	host := u.Hostname()
	if browserAllowPrivate {
		return true
	}
	if v, ok := s.hostsOK.Load(host); ok {
		return v.(bool)
	}
	_, err = vetHost(host)
	ok := err == nil
	s.hostsOK.Store(host, ok)
	return ok
}

// OpenURL checks a URL the agent or person asked for before loading it.
func browsableURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw != "" && !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", fmt.Errorf("only web addresses can be opened")
	}
	if u.User != nil {
		return "", fmt.Errorf("URLs with credentials are refused")
	}
	if !browserAllowPrivate {
		if _, err := vetHost(u.Hostname()); err != nil {
			return "", err
		}
	}
	return u.String(), nil
}

// run executes actions with a deadline and remembers the session was used.
func (s *BrowserSession) run(d time.Duration, actions ...chromedp.Action) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = time.Now()
	ctx, cancel := context.WithTimeout(s.ctx, d)
	defer cancel()
	return chromedp.Run(ctx, actions...)
}

func settle() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		// A page that keeps loading still gets read after a short wait.
		wait, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		chromedp.WaitReady("body", chromedp.ByQuery).Do(wait)
		select {
		case <-time.After(1200 * time.Millisecond):
		case <-ctx.Done():
		}
		return nil
	})
}

// snapshotJS labels what can be used on the page and returns it with the
// page's text, so a model that cannot see can still act on the page.
const snapshotJS = `(() => {
  document.querySelectorAll('[data-lamdis]').forEach(e => e.removeAttribute('data-lamdis'));
  const vis = e => { const r = e.getBoundingClientRect(), s = getComputedStyle(e);
    return r.width > 0 && r.height > 0 && s.visibility !== 'hidden' && s.display !== 'none' };
  const sel = 'a[href],button,input:not([type=hidden]),select,textarea,[role=button],[role=link],[role=checkbox],[role=radio],[role=tab],[role=menuitem],[role=option],[role=switch],[contenteditable=true],summary';
  const out = []; let n = 0;
  for (const e of document.querySelectorAll(sel)) {
    if (n >= 150) break;
    if (!vis(e)) continue;
    n++; e.setAttribute('data-lamdis', String(n));
    const tag = e.tagName.toLowerCase(), type = (e.getAttribute('type') || '').toLowerCase();
    let label = e.getAttribute('aria-label') || (e.labels && e.labels[0] && e.labels[0].innerText) ||
      e.innerText || e.getAttribute('placeholder') || e.getAttribute('title') || e.getAttribute('alt') || e.getAttribute('name') || '';
    label = label.replace(/\s+/g, ' ').trim().slice(0, 80);
    let kind = e.getAttribute('role') || tag;
    if (tag === 'input') kind = 'input' + (type ? '[' + type + ']' : '');
    let extra = '';
    if (tag === 'a') { try { const u = new URL(e.href); extra = ' -> ' + (u.host === location.host ? u.pathname : u.host + u.pathname).slice(0, 60) } catch (_) {} }
    if (tag === 'input' || tag === 'textarea') { const v = type === 'password' ? (e.value ? '••••' : '') : (e.value || ''); if (v) extra += ' value="' + v.slice(0, 40) + '"'; if (e.checked) extra += ' checked' }
    if (tag === 'select') extra = ' value="' + ((e.options[e.selectedIndex] || {}).text || '') + '" options: ' + [...e.options].slice(0, 12).map(o => o.text.trim()).join(' | ');
    out.push('[' + n + '] ' + kind + ' "' + label + '"' + extra);
  }
  const text = (document.body ? document.body.innerText : '').replace(/\n{3,}/g, '\n\n').trim();
  return JSON.stringify({title: document.title, url: location.href, text: text.slice(0, 7000), more: text.length > 7000, items: out});
})()`

type pageSnap struct {
	Title string   `json:"title"`
	URL   string   `json:"url"`
	Text  string   `json:"text"`
	More  bool     `json:"more"`
	Items []string `json:"items"`
}

// Read returns the page as the agent sees it.
func (s *BrowserSession) Read() (string, error) {
	var raw string
	if err := s.run(20*time.Second, chromedp.Evaluate(snapshotJS, &raw)); err != nil {
		return "", err
	}
	var p pageSnap
	json.Unmarshal([]byte(raw), &p)
	var b strings.Builder
	b.WriteString("Page: " + p.Title + " — " + p.URL + "\n\n" + p.Text)
	if p.More {
		b.WriteString("\n…(the page goes on; scroll to read more)")
	}
	if len(p.Items) > 0 {
		b.WriteString("\n\nThings you can use (by number):\n" + strings.Join(p.Items, "\n"))
	}
	s.fmu.Lock()
	refused := s.refused
	s.refused = 0
	s.fmu.Unlock()
	if refused > 0 {
		b.WriteString(fmt.Sprintf("\n\nNote: this site refused %d of its own requests from this browser. Sites that block automated browsers look like this, so a form here may not save even when it looks filled in. Do not tell the person a step went through unless the page confirms it; if it keeps happening, say the site is blocking the agent's browser and give them the direct link to do it themselves.", refused))
	}
	s.saveCookies()
	return b.String(), nil
}

// URL is where the browser is now.
func (s *BrowserSession) URL() (string, string) {
	var u, t string
	s.run(5*time.Second, chromedp.Location(&u), chromedp.Title(&t))
	return u, t
}

func item(id int) string { return fmt.Sprintf(`[data-lamdis="%d"]`, id) }

// Open loads a page.
func (s *BrowserSession) Open(raw string) error {
	u, err := browsableURL(raw)
	if err != nil {
		return err
	}
	return s.run(45*time.Second, chromedp.Navigate(u), settle())
}

// Click presses a numbered item from the last read.
func (s *BrowserSession) Click(id int) error {
	return s.run(30*time.Second, chromedp.ScrollIntoView(item(id), chromedp.ByQuery),
		chromedp.Click(item(id), chromedp.ByQuery), settle())
}

// Type replaces a field's text, and presses Enter when submit is set.
func (s *BrowserSession) Type(id int, text string, submit bool) error {
	sel := item(id)
	acts := []chromedp.Action{chromedp.ScrollIntoView(sel, chromedp.ByQuery), chromedp.Focus(sel, chromedp.ByQuery),
		chromedp.Evaluate(fmt.Sprintf(`(()=>{const e=document.querySelector('%s');if(e&&e.select)e.select();else if(e&&e.isContentEditable){document.execCommand('selectAll')}})()`, sel), nil),
		chromedp.SendKeys(sel, kb.Delete, chromedp.ByQuery),
		chromedp.SendKeys(sel, text, chromedp.ByQuery)}
	if submit {
		acts = append(acts, chromedp.SendKeys(sel, kb.Enter, chromedp.ByQuery))
	}
	acts = append(acts, settle())
	return s.run(30*time.Second, acts...)
}

// Choose picks an option in a numbered select by its visible text.
func (s *BrowserSession) Choose(id int, option string) error {
	js := fmt.Sprintf(`(()=>{const e=document.querySelector('%s');if(!e)return false;const o=[...e.options].find(o=>o.text.trim().toLowerCase()===%q.toLowerCase())||[...e.options].find(o=>o.text.toLowerCase().includes(%q.toLowerCase()));if(!o)return false;e.value=o.value;e.dispatchEvent(new Event('input',{bubbles:true}));e.dispatchEvent(new Event('change',{bubbles:true}));return true})()`, item(id), option, option)
	var ok bool
	if err := s.run(15*time.Second, chromedp.Evaluate(js, &ok), settle()); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no option like %q there", option)
	}
	return nil
}

// Scroll moves the page by about a screen.
func (s *BrowserSession) Scroll(down bool) error {
	dy := 700
	if !down {
		dy = -700
	}
	return s.run(10*time.Second, chromedp.Evaluate(fmt.Sprintf("window.scrollBy(0,%d)", dy), nil), chromedp.Sleep(500*time.Millisecond))
}

// Back goes to the previous page.
func (s *BrowserSession) Back() error {
	return s.run(20*time.Second, chromedp.NavigateBack(), settle())
}

// Screenshot is the live view the person sees when they take over.
func (s *BrowserSession) Screenshot() ([]byte, error) {
	var buf []byte
	err := s.run(15*time.Second, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		buf, err = page.CaptureScreenshot().WithFormat(page.CaptureScreenshotFormatJpeg).WithQuality(70).Do(ctx)
		return err
	}))
	return buf, err
}

// ClickAt presses a point on the live view (CSS pixels of the 1280x800 page).
func (s *BrowserSession) ClickAt(x, y float64) error {
	return s.run(15*time.Second, chromedp.MouseClickXY(x, y), chromedp.Sleep(600*time.Millisecond))
}

// Keys types into whatever has focus. Special keys: Enter, Backspace, Tab.
func (s *BrowserSession) Keys(text string) error {
	switch text {
	case "Enter":
		text = kb.Enter
	case "Backspace":
		text = kb.Backspace
	case "Tab":
		text = kb.Tab
	}
	return s.run(15*time.Second, chromedp.KeyEvent(text), chromedp.Sleep(300*time.Millisecond))
}

// TypeText types literally into whatever has focus.
func (s *BrowserSession) TypeText(text string) error {
	return s.run(30*time.Second, chromedp.KeyEvent(text), chromedp.Sleep(200*time.Millisecond))
}

// Save keeps the session's cookies now (after the person signs in).
func (s *BrowserSession) Save() { s.saveCookies() }

// Forget clears the account's saved sign-ins and closes its browser.
func (p *BrowserPool) Forget(key string) {
	p.mu.Lock()
	if s := p.sessions[key]; s != nil {
		s.cancel()
		delete(p.sessions, key)
	}
	p.mu.Unlock()
	os.Remove(cookiePath(key))
	os.Remove(filepath.Join(key, "browser.last"))
}

func cookiePath(dir string) string { return filepath.Join(dir, "browser.sealed") }

func (s *BrowserSession) saveCookies() {
	var cookies []*network.Cookie
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		var err error
		cookies, err = network.GetCookies().Do(c)
		return err
	})); err != nil || len(cookies) == 0 {
		return
	}
	raw, _ := json.Marshal(cookies)
	key, err := vaultKey(s.key)
	if err != nil {
		return
	}
	sealed, err := seal(key, string(raw))
	if err != nil {
		return
	}
	os.WriteFile(cookiePath(s.key), []byte(sealed), 0o600)
	var u string
	if chromedp.Run(ctx, chromedp.Location(&u)) == nil && strings.HasPrefix(u, "http") {
		if su, err := seal(key, u); err == nil {
			os.WriteFile(filepath.Join(s.key, "browser.last"), []byte(su), 0o600)
		}
	}
}

// lastURL is where this account's browser was, if it was there recently.
func (s *BrowserSession) lastURL() string {
	path := filepath.Join(s.key, "browser.last")
	fi, err := os.Stat(path)
	if err != nil || time.Since(fi.ModTime()) > 3*time.Hour {
		return ""
	}
	raw, _ := os.ReadFile(path)
	key, err := vaultKey(s.key)
	if err != nil {
		return ""
	}
	return unseal(key, string(raw))
}

func (s *BrowserSession) restoreCookies(ctx context.Context) error {
	raw, err := os.ReadFile(cookiePath(s.key))
	if err != nil {
		return nil
	}
	key, err := vaultKey(s.key)
	if err != nil {
		return nil
	}
	var cookies []*network.Cookie
	if json.Unmarshal([]byte(unseal(key, string(raw))), &cookies) != nil {
		return nil
	}
	var params []*network.CookieParam
	for _, c := range cookies {
		if c.Expires > 0 && time.Unix(int64(c.Expires), 0).Before(time.Now()) {
			continue
		}
		p := &network.CookieParam{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path,
			Secure: c.Secure, HTTPOnly: c.HTTPOnly, SameSite: c.SameSite}
		if c.Expires > 0 {
			t := cdp.TimeSinceEpoch(time.Unix(int64(c.Expires), 0))
			p.Expires = &t
		}
		params = append(params, p)
	}
	if len(params) == 0 {
		return nil
	}
	return network.SetCookies(params).Do(ctx)
}
