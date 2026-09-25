package agent

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// A team: more than one agent, each with its own name, manner and expertise,
// and optionally its own model and its own subset of connections.
//
// They are one identity to the protocol: every one of them signs with the
// agent key under the person's delegation, so adding a researcher or a
// negotiator needs no new grants and cannot reach further than the agent
// already could. What differs is who answers: the persona's name is written
// into everything it produces, so a reader can tell them apart.

// Persona is one member of the team.
type Persona struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	About string `json:"about"` // personality, expertise, how it should work
	// Model, when set, is used for this persona's runs (within whatever the
	// node allows). Empty means the agent's model.
	Model string `json:"model,omitempty"`
	// Tools, when set, limits which connections this persona may use, by
	// connection name. Empty means all of the agent's connections.
	Tools []string `json:"tools,omitempty"`
}

// NewPersonaID is a short random id.
func NewPersonaID() string {
	b := make([]byte, 5)
	rand.Read(b)
	return "p" + hex.EncodeToString(b)
}

// PersonaByID finds a persona, or nil.
func (c Config) PersonaByID(id string) *Persona {
	for i := range c.Agents {
		if c.Agents[i].ID == id {
			return &c.Agents[i]
		}
	}
	return nil
}

// PersonaMentioned returns the id of the first persona addressed with
// @Name in text, or "" when none is (the main agent answers then).
func PersonaMentioned(c Config, text string) string {
	low := strings.ToLower(text)
	best, at := "", -1
	for _, p := range c.Agents {
		n := strings.ToLower(strings.TrimSpace(p.Name))
		if n == "" {
			continue
		}
		for i := 0; ; {
			j := strings.Index(low[i:], "@"+n)
			if j < 0 {
				break
			}
			j += i
			end := j + 1 + len(n)
			if end == len(low) || !isWordByte(low[end]) {
				if at < 0 || j < at {
					best, at = p.ID, j
				}
				break
			}
			i = j + 1
		}
	}
	return best
}

// mayUseServer applies a persona's connection limit.
func (p *Persona) mayUseServer(server string) bool {
	if p == nil || len(p.Tools) == 0 {
		return true
	}
	for _, t := range p.Tools {
		if t == server {
			return true
		}
	}
	return false
}
