package agent

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/store"
)

// The agent has its own key. The protocol already knows how to bind an agent
// key to a person (core.delegation) and how to sever it, and every reader of
// the log can then tell "Sterling" from "Sterling's agent" without trusting a
// label. Nothing minted such a key before; this does.

func keyPath(dataDir string) string { return filepath.Join(dataDir, "agent.key") }

// LoadOrMintAgentKey returns the agent's key, creating it on first use. Same
// hex-seed format as the person key, same permissions.
func LoadOrMintAgentKey(dataDir string) (ed25519.PrivateKey, string, error) {
	raw, err := os.ReadFile(keyPath(dataDir))
	if os.IsNotExist(err) {
		_, priv, err := protolog.GenerateKeypair()
		if err != nil {
			return nil, "", err
		}
		if err := os.WriteFile(keyPath(dataDir), []byte(hex.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
			return nil, "", err
		}
		raw = []byte(hex.EncodeToString(priv.Seed()))
	} else if err != nil {
		return nil, "", err
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, "", fmt.Errorf("agent key at %s is corrupt", keyPath(dataDir))
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pid, err := protolog.PrincipalID(priv.Public().(ed25519.PublicKey))
	if err != nil {
		return nil, "", err
	}
	return priv, pid, nil
}

// HasAgentKey reports whether a key exists without creating one.
func HasAgentKey(dataDir string) bool {
	_, err := os.Stat(keyPath(dataDir))
	return err == nil
}

// EnsureDelegation binds the agent to the person in one thread, once. The
// entry goes in the control lane, signed by the person, so it replicates at
// every scope and a peer can verify the agent's writes.
func EnsureDelegation(ctx context.Context, s store.Store, personKey ed25519.PrivateKey, person, agent, thread string) error {
	tl, err := s.Thread(ctx, thread)
	if err != nil {
		return err
	}
	st := perm.Fold(thread, tl.Entries())
	if st.ActsFor(agent, person) {
		return nil
	}
	author, err := protolog.NewAuthor(tl, personKey)
	if err != nil {
		return err
	}
	e, err := author.Append(protolog.Draft{Kind: protolog.KindDelegation, Lane: protolog.LaneControl,
		Body: map[string]any{"agent": agent, "note": "built-in agent"}})
	if err != nil {
		return err
	}
	return s.AppendEntries(ctx, []*protolog.Entry{e})
}

// RevokeAgent severs the agent in every thread it was bound to and removes
// its key. A fresh key is minted on the next serve; the old one can never
// act again because the revocation is in the control lane of every thread.
func RevokeAgent(ctx context.Context, s store.Store, dataDir string, personKey ed25519.PrivateKey, person, agent string) (int, error) {
	ids, err := s.Threads(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		tl, err := s.Thread(ctx, id)
		if err != nil {
			continue
		}
		st := perm.Fold(id, tl.Entries())
		if !st.ActsFor(agent, person) || agent == person {
			continue
		}
		author, err := protolog.NewAuthor(tl, personKey)
		if err != nil {
			return n, err
		}
		e, err := author.Append(protolog.Draft{Kind: protolog.KindDelegation, Lane: protolog.LaneControl,
			Body: map[string]any{"agent": agent, "revoked": true}})
		if err != nil {
			return n, err
		}
		if err := s.AppendEntries(ctx, []*protolog.Entry{e}); err != nil {
			return n, err
		}
		n++
	}
	if err := os.Remove(keyPath(dataDir)); err != nil && !os.IsNotExist(err) {
		return n, err
	}
	return n, nil
}
