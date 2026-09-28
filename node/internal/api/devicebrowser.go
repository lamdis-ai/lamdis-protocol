package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// Your own computer's browser, driven by your agents.
//
// A small helper on the person's machine (lamdis browser) opens Chrome with a
// Lamdis profile and dials out to this host over a WebSocket. That socket
// carries the Chrome DevTools protocol, unchanged. The agent's browser code
// connects to a loopback address here, and the hub splices it onto the
// device's socket, so the same code drives a cloud Chrome or the person's
// own, where they are signed in and on their own connection.
//
// One message is not DevTools: {"lamdis":"reset"} asks the helper to open a
// fresh connection to its Chrome, sent whenever the agent side reconnects,
// because DevTools state belongs to a connection.

type deviceHub struct {
	mu    sync.Mutex
	links map[string]*deviceLink // account -> its connected browser
	addr  string                 // loopback listener for the agent side
	once  sync.Once
}

type deviceLink struct {
	conn    net.Conn
	wmu     sync.Mutex
	inbox   chan []byte // device -> agent side
	busy    bool        // an agent-side connection is attached
	since   time.Time
	version string
}

func (l *deviceLink) send(msg []byte) error {
	l.wmu.Lock()
	defer l.wmu.Unlock()
	l.conn.SetWriteDeadline(time.Now().Add(20 * time.Second))
	return wsutil.WriteServerMessage(l.conn, ws.OpText, msg)
}

func newDeviceHub() *deviceHub { return &deviceHub{links: map[string]*deviceLink{}} }

// start opens the loopback listener the agent's browser code dials.
func (d *deviceHub) start() {
	d.once.Do(func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return
		}
		d.addr = ln.Addr().String()
		srv := &http.Server{Handler: http.HandlerFunc(d.serveAgent)}
		go srv.Serve(ln)
	})
}

// CDP is the DevTools address for an account's own browser, when connected.
func (d *deviceHub) CDP(acct string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.links[acct] == nil || d.addr == "" {
		return ""
	}
	return "ws://" + d.addr + "/dev/" + acct
}

func (d *deviceHub) connected(acct string) (bool, time.Time, string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	l := d.links[acct]
	if l == nil {
		return false, time.Time{}, ""
	}
	return true, l.since, l.version
}

// GET /app/api/device/browser — the helper's socket. Authenticated as the
// account like any other call.
func (h *Host) handleDeviceBrowser(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		return
	}
	d := h.devices
	d.start()
	l := &deviceLink{conn: conn, inbox: make(chan []byte, 256), since: time.Now(), version: r.Header.Get("X-Lamdis-Browser")}
	d.mu.Lock()
	if old := d.links[me.ID]; old != nil {
		old.conn.Close() // one computer at a time; the newest wins
	}
	d.links[me.ID] = l
	d.mu.Unlock()
	h.logf("host: %s: own browser connected (%s)", me.ID, l.version)
	defer func() {
		d.mu.Lock()
		if d.links[me.ID] == l {
			delete(d.links, me.ID)
		}
		d.mu.Unlock()
		conn.Close()
		close(l.inbox)
		if h.Browser != nil {
			h.Browser.DropDevice(filepath.Join(h.Root, me.ID))
		}
		h.logf("host: %s: own browser disconnected", me.ID)
	}()
	// Keep proxies and load balancers from timing out a quiet socket.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				l.wmu.Lock()
				wsutil.WriteServerMessage(conn, ws.OpPing, nil)
				l.wmu.Unlock()
			}
		}
	}()
	for {
		msg, op, err := wsutil.ReadClientData(conn)
		if err != nil {
			return
		}
		if op != ws.OpText {
			continue
		}
		select {
		case l.inbox <- msg:
		default: // nobody attached and the buffer is full: drop, the agent side resets anyway
		}
	}
}

// serveAgent splices one agent-side DevTools connection onto the device.
func (d *deviceHub) serveAgent(w http.ResponseWriter, r *http.Request) {
	acct, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/dev/"), "/")
	d.mu.Lock()
	l := d.links[acct]
	if l != nil && l.busy {
		l = nil
	}
	if l != nil {
		l.busy = true
	}
	d.mu.Unlock()
	if l == nil {
		http.Error(w, "no browser", http.StatusServiceUnavailable)
		return
	}
	defer func() { d.mu.Lock(); l.busy = false; d.mu.Unlock() }()
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		return
	}
	defer conn.Close()
	// A fresh DevTools connection on the device for this agent connection.
	for len(l.inbox) > 0 {
		<-l.inbox
	}
	reset, _ := json.Marshal(map[string]string{"lamdis": "reset"})
	if l.send(reset) != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-l.inbox:
				if !ok {
					return
				}
				if wsutil.WriteServerMessage(conn, ws.OpText, msg) != nil {
					return
				}
			}
		}
	}()
	for {
		msg, op, err := wsutil.ReadClientData(conn)
		if err != nil || ctx.Err() != nil {
			return
		}
		if op == ws.OpText && l.send(msg) != nil {
			return
		}
	}
}

// GET /app/api/device/browser/status -> {connected, since, version}
func (h *Host) handleDeviceBrowserStatus(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	ok, since, v := h.devices.connected(me.ID)
	out := map[string]any{"connected": ok}
	if ok {
		out["since"] = since.UTC().Format(time.RFC3339)
		out["version"] = v
	}
	writeJSON(w, out)
}
