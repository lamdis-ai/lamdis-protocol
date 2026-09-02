// Package anchor ties receipts to a public chain, so a receipt can be shown
// to have existed unchanged at a point in time without trusting this exchange
// — or needing it to still be running.
//
// Receipt hashes are collected as receipts are issued, batched into a Merkle
// tree on a timer, and the root is submitted to OpenTimestamps calendars,
// which aggregate it into a Bitcoin transaction. What comes back is an .ots
// proof anybody can check with the public `ots` tool. The exchange keeps the
// hashes, the trees and the proofs; it never needs a key or a wallet.
package anchor

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// Step is one hop of an inclusion path: the sibling hash and which side of
// the pair it sits on.
type Step struct {
	Sibling string `json:"sibling"`
	// Side is "left" when the sibling is concatenated before the running
	// hash, "right" when after.
	Side string `json:"side"`
}

// merkleRoot folds hex-encoded SHA-256 leaves into a root.
//
// Pairs are hashed as SHA-256(left || right). An odd node at any level is
// carried up unchanged rather than duplicated, so a tree with one leaf has
// that leaf as its root and an inclusion path of no steps. Leaves keep the
// order they were given; nothing is sorted, so a path is a path through the
// tree as built.
func merkleRoot(leaves []string) (string, error) {
	level, err := decodeLeaves(leaves)
	if err != nil {
		return "", err
	}
	for len(level) > 1 {
		level = nextLevel(level)
	}
	return hex.EncodeToString(level[0]), nil
}

// merklePath is the inclusion path of leaves[i].
func merklePath(leaves []string, i int) ([]Step, error) {
	level, err := decodeLeaves(leaves)
	if err != nil {
		return nil, err
	}
	if i < 0 || i >= len(level) {
		return nil, errors.New("anchor: leaf index out of range")
	}
	path := []Step{}
	for len(level) > 1 {
		if i%2 == 0 {
			if i+1 < len(level) {
				path = append(path, Step{Sibling: hex.EncodeToString(level[i+1]), Side: "right"})
			}
			// An odd node on the end pairs with nothing and is carried up.
		} else {
			path = append(path, Step{Sibling: hex.EncodeToString(level[i-1]), Side: "left"})
		}
		level = nextLevel(level)
		i /= 2
	}
	return path, nil
}

// VerifyPath folds a leaf along its path and reports whether it reaches root.
// This is what a third party does with the numbers on the anchor endpoint.
func VerifyPath(leaf string, path []Step, root string) bool {
	h, err := hex.DecodeString(leaf)
	if err != nil || len(h) != sha256.Size {
		return false
	}
	for _, st := range path {
		sib, err := hex.DecodeString(st.Sibling)
		if err != nil || len(sib) != sha256.Size {
			return false
		}
		switch st.Side {
		case "left":
			h = pairHash(sib, h)
		case "right":
			h = pairHash(h, sib)
		default:
			return false
		}
	}
	return hex.EncodeToString(h) == root
}

func nextLevel(level [][]byte) [][]byte {
	next := make([][]byte, 0, (len(level)+1)/2)
	for j := 0; j < len(level); j += 2 {
		if j+1 < len(level) {
			next = append(next, pairHash(level[j], level[j+1]))
		} else {
			next = append(next, level[j])
		}
	}
	return next
}

func pairHash(l, r []byte) []byte {
	h := sha256.New()
	h.Write(l)
	h.Write(r)
	return h.Sum(nil)
}

func decodeLeaves(leaves []string) ([][]byte, error) {
	if len(leaves) == 0 {
		return nil, errors.New("anchor: a tree needs at least one leaf")
	}
	out := make([][]byte, len(leaves))
	for i, l := range leaves {
		b, err := hex.DecodeString(l)
		if err != nil || len(b) != sha256.Size {
			return nil, errors.New("anchor: leaf " + l + " is not a hex SHA-256")
		}
		out[i] = b
	}
	return out, nil
}
