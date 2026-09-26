package agent

import (
	"sync"
	"time"
)

// Pool is a host's daily ceiling on what every account spending its shared
// model key may use together. Each account has its own allowance; the pool
// is what keeps a thousand generous allowances from becoming one large bill.
// It counts in memory and restarts at zero with the process, which errs on
// the side of letting people keep working.
type Pool struct {
	Tokens int // 0 means no ceiling

	mu   sync.Mutex
	day  string
	used int
}

func (p *Pool) roll(now time.Time) {
	if d := now.UTC().Format("2006-01-02"); d != p.day {
		p.day, p.used = d, 0
	}
}

// Full reports whether today's shared tokens are spent.
func (p *Pool) Full(now time.Time) bool {
	if p == nil || p.Tokens <= 0 {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.roll(now)
	return p.used >= p.Tokens
}

// Add counts tokens spent on the shared key.
func (p *Pool) Add(now time.Time, n int) {
	if p == nil || n <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.roll(now)
	p.used += n
}
