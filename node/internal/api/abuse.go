package api

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
)

// Starting without signing up, without being farmed.
//
// Anybody can start as a guest, which is the point, and it is also the
// obvious thing for a script to abuse: mint accounts, spend the shared model
// key. Three things keep that bounded without asking people to sign up
// before their first question:
//
//   - New guests per network are limited (a few an hour, a handful a day).
//   - A guest gets a small daily allowance. Confirming an email (a code sent
//     to it, one account per address) or saving a passkey makes the account
//     one somebody can come back to, and it gets the full allowance.
//   - Guests and kept accounts spend from separate daily pools, so however
//     many guests a script makes, it cannot use up what people who have
//     confirmed are relying on.

var (
	startMu  sync.Mutex
	startLog = map[string][]time.Time{}
)

// clientIP is the visitor's address as the edge saw it. CloudFront adds its
// own hop to X-Forwarded-For after the viewer's, so the first entry is the
// person; a /64 stands for one IPv6 network.
func clientIP(r *http.Request) string {
	ip := ""
	if v := r.Header.Get("CloudFront-Viewer-Address"); v != "" {
		if h, _, err := net.SplitHostPort(v); err == nil {
			ip = h
		} else if i := strings.LastIndex(v, ":"); i > 0 {
			ip = v[:i]
		}
	}
	if ip == "" {
		if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
			ip = strings.TrimSpace(strings.Split(xf, ",")[0])
		}
	}
	if ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	if p := net.ParseIP(ip); p != nil && p.To4() == nil {
		return p.Mask(net.CIDRMask(64, 128)).String() + "/64"
	}
	return ip
}

// allowStart records a new guest from ip, or says it is one too many.
// Zero limits mean no limit.
func allowStart(ip string, now time.Time, perHour, perDay int) bool {
	if perHour <= 0 && perDay <= 0 {
		return true
	}
	startMu.Lock()
	defer startMu.Unlock()
	var day []time.Time
	hour := 0
	for _, t := range startLog[ip] {
		if now.Sub(t) < 24*time.Hour {
			day = append(day, t)
			if now.Sub(t) < time.Hour {
				hour++
			}
		}
	}
	if (perDay > 0 && len(day) >= perDay) || (perHour > 0 && hour >= perHour) {
		startLog[ip] = day
		return false
	}
	startLog[ip] = append(day, now)
	return true
}

// keptAccount: there is a way back into it from somewhere else, which is
// what a script minting throwaway accounts does not bother to make.
func keptAccount(dir string) bool {
	if raw, err := os.ReadFile(filepath.Join(dir, "email_confirmed")); err == nil && len(strings.TrimSpace(string(raw))) > 0 {
		return true
	}
	if len(readCreds(dir)) > 0 {
		return true
	}
	_, err := os.Stat(filepath.Join(dir, "subject"))
	return err == nil
}

// poolFor is the daily pool an account spends the shared key from.
func (h *Host) poolFor(dir string) *agent.Pool {
	if h.GuestPool != nil && !keptAccount(dir) {
		return h.GuestPool
	}
	return h.Pool
}

// promote gives an account that has just become kept its full allowance.
func (h *Host) promote(a *Account) {
	if a == nil {
		return
	}
	h.budget(a.Dir)
	if a.App != nil && a.App.Runner != nil {
		a.App.Runner.Pool = h.poolFor(a.Dir)
		a.App.Runner.Guest = false
	}
}
