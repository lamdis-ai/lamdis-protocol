package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// lamdis browser: lend this computer's Chrome to your agents.
//
//	lamdis browser ABCD-2345   first time: the code from Add a device
//	lamdis browser             after that
//
// Chrome opens with its own Lamdis profile, in a window you can watch. Sign
// in to the sites you want your agents to use, once, in that window. Your
// agents then browse there, on your own connection, instead of in the cloud.
// Nothing listens on this computer: the helper dials out to your host.

const browserHelperVersion = "1"

type browserCreds struct {
	Host  string `json:"host"`
	Token string `json:"token"`
}

func browserCredsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lamdis", "browser.json")
}

func runBrowser(args []string) error {
	host := "https://app.lamdis.ai"
	var code string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--host" && i+1 < len(args):
			host = strings.TrimRight(args[i+1], "/")
			i++
		case strings.HasPrefix(a, "--host="):
			host = strings.TrimRight(strings.TrimPrefix(a, "--host="), "/")
		case !strings.HasPrefix(a, "-"):
			code = a
		}
	}
	creds, _ := loadBrowserCreds()
	if code != "" {
		tok, err := redeemDeviceCode(host, code)
		if err != nil {
			return err
		}
		creds = browserCreds{Host: host, Token: tok}
		if err := saveBrowserCreds(creds); err != nil {
			return err
		}
	}
	if creds.Token == "" {
		fmt.Println("First, get a code: in Lamdis open Settings (or You on the phone), choose")
		fmt.Println("\"Use this computer's browser\", and run the command it shows, e.g.")
		fmt.Println("    lamdis browser ABCD-2345")
		return nil
	}
	chrome := findChrome()
	if chrome == "" {
		return errors.New("could not find Google Chrome; install it and run this again")
	}
	home, _ := os.UserHomeDir()
	profile := filepath.Join(home, ".lamdis", "chrome-profile")
	os.MkdirAll(profile, 0o700)
	port, err := freePort()
	if err != nil {
		return err
	}
	chromeArgs := []string{fmt.Sprintf("--remote-debugging-port=%d", port), "--remote-debugging-address=127.0.0.1",
		"--user-data-dir=" + profile, "--no-first-run", "--no-default-browser-check"}
	if os.Getenv("LAMDIS_BROWSER_HEADLESS") != "" { // for tests; people get a window they can watch
		chromeArgs = append(chromeArgs, "--headless=new")
	}
	cmd := exec.Command(chrome, append(chromeArgs, "about:blank")...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start Chrome: %v", err)
	}
	defer cmd.Process.Kill()
	local := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	if err := waitFor(local, 30*time.Second); err != nil {
		return fmt.Errorf("Chrome did not open its DevTools port: %v", err)
	}
	fmt.Println("Chrome is open with your Lamdis profile. Sign in to sites there once;")
	fmt.Println("your agents will use this window while this command runs. Ctrl-C to stop.")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() { cmd.Wait(); cancel() }() // closing Chrome ends the helper
	wait := time.Second
	for ctx.Err() == nil {
		start := time.Now()
		err := relayBrowser(ctx, creds, local)
		if errors.Is(err, errSignIn) {
			return errors.New("that sign-in has expired: get a new code in Lamdis and run lamdis browser CODE")
		}
		if ctx.Err() != nil {
			break
		}
		if time.Since(start) > time.Minute {
			wait = time.Second
		}
		fmt.Printf("connection to %s dropped (%v); retrying in %s\n", creds.Host, err, wait)
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
		if wait < 30*time.Second {
			wait *= 2
		}
	}
	return nil
}

var errSignIn = errors.New("sign-in expired")

// relayBrowser carries DevTools traffic between the host and local Chrome
// until either side goes away.
func relayBrowser(ctx context.Context, creds browserCreds, localVersion string) error {
	u := strings.Replace(creds.Host, "https://", "wss://", 1)
	u = strings.Replace(u, "http://", "ws://", 1) + "/app/api/device/browser"
	d := ws.Dialer{Header: ws.HandshakeHeaderHTTP(http.Header{
		"Authorization":    {"Bearer " + creds.Token},
		"X-Lamdis-Browser": {browserHelperVersion + " " + runtime.GOOS},
	}), Timeout: 20 * time.Second}
	conn, br, _, err := d.Dial(ctx, u)
	if err != nil {
		var se ws.StatusError
		if errors.As(err, &se) && int(se) == http.StatusUnauthorized {
			return errSignIn
		}
		return err
	}
	defer conn.Close()
	var server io.ReadWriter = conn
	if br != nil {
		server = rw{io.MultiReader(br, conn), conn}
	}
	fmt.Println("Connected: your agents can browse here.")

	var wmu sync.Mutex // writes to the host
	toHost := func(msg []byte) error {
		wmu.Lock()
		defer wmu.Unlock()
		return wsutil.WriteClientMessage(conn, ws.OpText, msg)
	}
	var lmu sync.Mutex
	var localConn net.Conn
	closeLocal := func() {
		lmu.Lock()
		if localConn != nil {
			localConn.Close()
			localConn = nil
		}
		lmu.Unlock()
	}
	defer closeLocal()
	openLocal := func() error {
		closeLocal()
		wsURL, err := browserWS(localVersion)
		if err != nil {
			return err
		}
		lc, lbr, _, err := ws.Dial(ctx, wsURL)
		if err != nil {
			return err
		}
		var lr io.ReadWriter = lc
		if lbr != nil {
			lr = rw{io.MultiReader(lbr, lc), lc}
		}
		lmu.Lock()
		localConn = lc
		lmu.Unlock()
		go func() {
			for {
				msg, op, err := wsutil.ReadServerData(lr)
				if err != nil {
					return
				}
				if op == ws.OpText {
					if toHost(msg) != nil {
						return
					}
				}
			}
		}()
		return nil
	}
	go func() { <-ctx.Done(); conn.Close() }()
	for {
		msgs, err := wsutil.ReadServerMessage(server, nil)
		if err != nil {
			return err
		}
		for _, m := range msgs {
			switch m.OpCode {
			case ws.OpPing:
				wmu.Lock()
				wsutil.WriteClientMessage(conn, ws.OpPong, m.Payload)
				wmu.Unlock()
			case ws.OpClose:
				return errors.New("closed by host")
			case ws.OpText:
				if bytes.HasPrefix(bytes.TrimSpace(m.Payload), []byte(`{"lamdis"`)) {
					if err := openLocal(); err != nil {
						fmt.Println("could not reach Chrome:", err)
					}
					continue
				}
				lmu.Lock()
				lc := localConn
				lmu.Unlock()
				if lc != nil {
					wsutil.WriteClientMessage(lc, ws.OpText, m.Payload)
				}
			}
		}
	}
}

type rw struct {
	io.Reader
	io.Writer
}

func browserWS(versionURL string) (string, error) {
	resp, err := http.Get(versionURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var v struct {
		WS string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil || v.WS == "" {
		return "", fmt.Errorf("no DevTools address from Chrome")
	}
	return v.WS, nil
}

func redeemDeviceCode(host, code string) (string, error) {
	body, _ := json.Marshal(map[string]string{"code": code})
	resp, err := http.Post(host+"/app/api/device/redeem", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Token string `json:"token"`
		Error string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if out.Token == "" {
		if out.Error == "" {
			out.Error = resp.Status
		}
		return "", errors.New(out.Error)
	}
	return out.Token, nil
}

func loadBrowserCreds() (browserCreds, error) {
	var c browserCreds
	raw, err := os.ReadFile(browserCredsPath())
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(raw, &c)
	return c, err
}

func saveBrowserCreds(c browserCreds) error {
	os.MkdirAll(filepath.Dir(browserCredsPath()), 0o700)
	raw, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(browserCredsPath(), raw, 0o600)
}

func findChrome() string {
	var cands []string
	switch runtime.GOOS {
	case "darwin":
		cands = []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium"}
	case "windows":
		cands = []string{`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`}
	default:
		cands = []string{"/usr/bin/google-chrome", "/usr/bin/chromium", "/usr/bin/chromium-browser"}
	}
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func waitFor(url string, d time.Duration) error {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(url); err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.New("timed out")
}
