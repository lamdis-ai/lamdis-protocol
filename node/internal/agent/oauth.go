package agent

// Connecting to a server that has real authentication.
//
// "Paste a token" covers the easy half of the world. The other half, and
// what the protocol itself specifies for anything served over HTTP, is
// OAuth: the server answers an unauthenticated call with a pointer to its
// authorization server, the client registers itself, the person approves in
// their browser, and what comes back is a short-lived token with a refresh
// beside it. Nobody types a secret at any point, which is the point.
//
// Four documents do the work and all of them are discovery: protected
// resource metadata says who guards this server, authorization server
// metadata says where to send people, dynamic client registration means we
// do not have to be pre-arranged with anybody, and PKCE means the code that
// comes back is only good to whoever asked for it.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuthConfig is everything learned about one server's authentication,
// plus whatever it has granted us so far.
type OAuthConfig struct {
	Issuer      string   `json:"issuer"`
	AuthURL     string   `json:"authorization_endpoint"`
	TokenURL    string   `json:"token_endpoint"`
	RegisterURL string   `json:"registration_endpoint,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	// Resource is the server's own canonical id, sent with every request so
	// a token minted for one service cannot be spent at another.
	Resource string `json:"resource,omitempty"`

	ClientID string    `json:"client_id,omitempty"`
	Access   string    `json:"access_token,omitempty"`
	Refresh  string    `json:"refresh_token,omitempty"`
	Expiry   time.Time `json:"expiry,omitempty"`
}

// Connected reports whether this server has granted anything yet.
func (o *OAuthConfig) Connected() bool { return o != nil && (o.Access != "" || o.Refresh != "") }

var oauthHTTP = &http.Client{Timeout: 20 * time.Second}

func getJSON(ctx context.Context, u string, out any, allowLocal bool) error {
	if _, err := PublicHost(u); err != nil && !(allowLocal && localURL(u)) {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s answered HTTP %d", u, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func localURL(u string) bool {
	return strings.HasPrefix(u, "http://localhost") || strings.HasPrefix(u, "http://127.0.0.1")
}

// DiscoverOAuth asks a server how it wants to be authenticated. It returns
// nil without an error when the server needs nothing, which is the common
// case and should not read as a failure.
func DiscoverOAuth(ctx context.Context, mcpURL string, allowLocal bool) (*OAuthConfig, error) {
	u, err := url.Parse(mcpURL)
	if err != nil {
		return nil, fmt.Errorf("that is not an address")
	}
	// The same rule as everywhere else: a node that runs on somebody's own
	// machine may reach that machine, and a hosted one may not reach its.
	if _, err := PublicHost(mcpURL); err != nil && !(allowLocal && localURL(mcpURL)) {
		return nil, err
	}
	// An unauthenticated call is the polite way to ask. A server that wants
	// OAuth answers 401 and names its metadata in the challenge.
	meta := ""
	req, _ := http.NewRequestWithContext(ctx, "POST", mcpURL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if resp, err := oauthHTTP.Do(req); err == nil {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			return nil, nil // it did not ask us to authenticate
		}
		meta = resourceMetadataURL(resp.Header.Get("WWW-Authenticate"))
	}
	if meta == "" {
		meta = u.Scheme + "://" + u.Host + "/.well-known/oauth-protected-resource"
	}

	var prm struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
		ScopesSupported      []string `json:"scopes_supported"`
	}
	issuer := ""
	if err := getJSON(ctx, meta, &prm, allowLocal); err == nil && len(prm.AuthorizationServers) > 0 {
		issuer = prm.AuthorizationServers[0]
	} else {
		// Some servers are their own authorization server and publish
		// nothing else; try that before giving up.
		issuer = u.Scheme + "://" + u.Host
	}

	cfg := &OAuthConfig{Issuer: issuer, Resource: prm.Resource, Scopes: prm.ScopesSupported}
	if cfg.Resource == "" {
		cfg.Resource = u.Scheme + "://" + u.Host + u.Path
	}
	var as struct {
		Issuer      string   `json:"issuer"`
		AuthURL     string   `json:"authorization_endpoint"`
		TokenURL    string   `json:"token_endpoint"`
		RegisterURL string   `json:"registration_endpoint"`
		Scopes      []string `json:"scopes_supported"`
	}
	var lastErr error
	for _, wk := range []string{"/.well-known/oauth-authorization-server", "/.well-known/openid-configuration"} {
		if err := getJSON(ctx, strings.TrimRight(issuer, "/")+wk, &as, allowLocal); err == nil && as.TokenURL != "" {
			lastErr = nil
			break
		} else if err != nil {
			lastErr = err
		}
	}
	if as.TokenURL == "" {
		if lastErr == nil {
			lastErr = fmt.Errorf("it did not say where to send people")
		}
		return nil, fmt.Errorf("this server wants you to sign in, but %w", lastErr)
	}
	cfg.AuthURL, cfg.TokenURL, cfg.RegisterURL = as.AuthURL, as.TokenURL, as.RegisterURL
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = as.Scopes
	}
	if as.Issuer != "" {
		cfg.Issuer = as.Issuer
	}
	return cfg, nil
}

// resourceMetadataURL pulls the pointer out of a challenge header.
func resourceMetadataURL(challenge string) string {
	for _, part := range strings.Split(challenge, ",") {
		part = strings.TrimSpace(part)
		if i := strings.Index(strings.ToLower(part), "resource_metadata="); i >= 0 {
			v := strings.Trim(part[i+len("resource_metadata="):], `" `)
			if strings.HasPrefix(v, "http") {
				return v
			}
		}
	}
	return ""
}

// Register introduces this node to the authorization server. Nobody has to
// arrange anything in advance, which is what makes connecting an arbitrary
// server possible at all.
func (o *OAuthConfig) Register(ctx context.Context, redirectURI, name string) error {
	if o.ClientID != "" {
		return nil
	}
	if o.RegisterURL == "" {
		return fmt.Errorf("this server needs a client id arranged in advance; it does not accept new ones automatically")
	}
	body, _ := json.Marshal(map[string]any{
		"client_name":                name,
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
		"application_type":           "web",
	})
	req, err := http.NewRequestWithContext(ctx, "POST", o.RegisterURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("registration was refused: %s", trunc(strings.TrimSpace(string(raw)), 200))
	}
	var out struct {
		ClientID string `json:"client_id"`
	}
	if json.Unmarshal(raw, &out) != nil || out.ClientID == "" {
		return fmt.Errorf("registration returned no client id")
	}
	o.ClientID = out.ClientID
	return nil
}

// Verifier is one PKCE secret, kept until the person comes back.
func Verifier() string {
	b := make([]byte, 48)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func challengeFor(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// Authorize is where to send the person's browser.
func (o *OAuthConfig) Authorize(redirectURI, verifier, state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", o.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("code_challenge", challengeFor(verifier))
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	if o.Resource != "" {
		q.Set("resource", o.Resource)
	}
	if len(o.Scopes) > 0 {
		q.Set("scope", strings.Join(o.Scopes, " "))
	}
	sep := "?"
	if strings.Contains(o.AuthURL, "?") {
		sep = "&"
	}
	return o.AuthURL + sep + q.Encode()
}

func (o *OAuthConfig) token(ctx context.Context, form url.Values) error {
	if o.Resource != "" {
		form.Set("resource", o.Resource)
	}
	form.Set("client_id", o.ClientID)
	req, err := http.NewRequestWithContext(ctx, "POST", o.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Access    string `json:"access_token"`
		Refresh   string `json:"refresh_token"`
		ExpiresIn int    `json:"expires_in"`
		Error     string `json:"error"`
		Desc      string `json:"error_description"`
	}
	json.Unmarshal(raw, &out)
	if resp.StatusCode >= 300 || out.Access == "" {
		msg := out.Desc
		if msg == "" {
			msg = out.Error
		}
		if msg == "" {
			msg = trunc(strings.TrimSpace(string(raw)), 200)
		}
		return fmt.Errorf("%s", msg)
	}
	o.Access = out.Access
	if out.Refresh != "" {
		o.Refresh = out.Refresh
	}
	secs := out.ExpiresIn
	if secs <= 0 {
		secs = 3600
	}
	o.Expiry = time.Now().Add(time.Duration(secs-60) * time.Second)
	return nil
}

// ExchangeCode turns what the browser came back with into a token.
func (o *OAuthConfig) ExchangeCode(ctx context.Context, code, verifier, redirectURI string) error {
	return o.token(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	})
}

// EnsureFresh renews the token when it is about to expire, so a connection
// made once keeps working without anybody thinking about it.
func (o *OAuthConfig) EnsureFresh(ctx context.Context) error {
	if o == nil || o.Access == "" && o.Refresh == "" {
		return nil
	}
	if o.Access != "" && time.Now().Before(o.Expiry) {
		return nil
	}
	if o.Refresh == "" {
		return fmt.Errorf("the connection expired and there is nothing to renew it with; connect it again")
	}
	return o.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {o.Refresh}})
}
