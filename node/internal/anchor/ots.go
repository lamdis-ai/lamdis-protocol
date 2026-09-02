package anchor

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

// The OpenTimestamps format, as much of it as it takes to store a proof,
// merge what several calendars return, follow a pending attestation to its
// Bitcoin one, and write a file the `ots` tool will read.
//
// This deliberately does not verify anything against Bitcoin. That is the
// verifier's job, done with a tool this exchange does not control — which is
// the whole point.

// otsMagic opens every detached .ots file.
var otsMagic = []byte("\x00OpenTimestamps\x00\x00Proof\x00\xbf\x89\xe2\xe8\x84\xe8\x92\x94")

const otsMajorVersion = 1

// Operation tags.
const (
	opSHA1      = 0x02
	opRIPEMD160 = 0x03
	opSHA256    = 0x08
	opKeccak256 = 0x67
	opAppend    = 0xf0
	opPrepend   = 0xf1
	opReverse   = 0xf2
	opHexlify   = 0xf3
)

// Attestation tags: eight bytes naming what kind of claim follows.
var (
	tagPending  = [8]byte{0x83, 0xdf, 0xe3, 0x0d, 0x2e, 0xf9, 0x0c, 0x8e}
	tagBitcoin  = [8]byte{0x05, 0x88, 0x96, 0x0d, 0x73, 0xd7, 0x19, 0x01}
	tagLitecoin = [8]byte{0x06, 0x86, 0x9a, 0x0d, 0x73, 0xd7, 0x1b, 0x45}
)

const (
	maxOpArg   = 4096
	maxURI     = 1000
	maxPayload = 8192
	maxDepth   = 256
)

// Op transforms a message on the way from a digest to an attestation.
type Op struct {
	Tag byte
	Arg []byte
}

// Apply computes the message this op produces.
func (o Op) Apply(msg []byte) ([]byte, error) {
	switch o.Tag {
	case opSHA256:
		h := sha256.Sum256(msg)
		return h[:], nil
	case opSHA1:
		h := sha1.Sum(msg)
		return h[:], nil
	case opAppend:
		return append(append([]byte{}, msg...), o.Arg...), nil
	case opPrepend:
		return append(append([]byte{}, o.Arg...), msg...), nil
	case opReverse:
		out := make([]byte, len(msg))
		for i, b := range msg {
			out[len(msg)-1-i] = b
		}
		return out, nil
	case opHexlify:
		return []byte(fmt.Sprintf("%x", msg)), nil
	case opRIPEMD160, opKeccak256:
		// Not in the standard library and not something a calendar emits on
		// the path we care about. The structure still parses; the message
		// below it is unknown, so nothing under it can be upgraded here.
		return nil, fmt.Errorf("ots: op %#x is not computed locally", o.Tag)
	}
	return nil, fmt.Errorf("ots: unknown op %#x", o.Tag)
}

func (o Op) less(p Op) bool {
	if o.Tag != p.Tag {
		return o.Tag < p.Tag
	}
	return bytes.Compare(o.Arg, p.Arg) < 0
}

func (o Op) equal(p Op) bool { return o.Tag == p.Tag && bytes.Equal(o.Arg, p.Arg) }

// Attestation is a claim about a message: that a calendar has it and will
// commit it (pending), or that it is committed under a block (bitcoin).
type Attestation struct {
	Tag     [8]byte
	Payload []byte
}

// Pending returns the calendar URI a pending attestation points at.
func (a Attestation) Pending() (string, bool) {
	if a.Tag != tagPending {
		return "", false
	}
	r := &reader{b: a.Payload}
	uri, err := r.varbytes(maxURI)
	if err != nil {
		return "", false
	}
	return string(uri), true
}

// Bitcoin returns the block height of a bitcoin attestation.
func (a Attestation) Bitcoin() (int, bool) {
	if a.Tag != tagBitcoin {
		return 0, false
	}
	r := &reader{b: a.Payload}
	h, err := r.varuint()
	if err != nil {
		return 0, false
	}
	return int(h), true
}

func (a Attestation) less(b Attestation) bool {
	if c := bytes.Compare(a.Tag[:], b.Tag[:]); c != 0 {
		return c < 0
	}
	return bytes.Compare(a.Payload, b.Payload) < 0
}

func (a Attestation) equal(b Attestation) bool {
	return a.Tag == b.Tag && bytes.Equal(a.Payload, b.Payload)
}

// pendingAttestation builds a pending attestation for a calendar.
func pendingAttestation(uri string) Attestation {
	var w writer
	w.varbytes([]byte(uri))
	return Attestation{Tag: tagPending, Payload: w.Bytes()}
}

// bitcoinAttestation builds a bitcoin block attestation.
func bitcoinAttestation(height int) Attestation {
	var w writer
	w.varuint(uint64(height))
	return Attestation{Tag: tagBitcoin, Payload: w.Bytes()}
}

// Timestamp is a tree of operations from one message to its attestations.
type Timestamp struct {
	Msg          []byte
	Attestations []Attestation
	Ops          []Branch
}

// Branch is an op and the timestamp of the message it produces.
type Branch struct {
	Op   Op
	Next *Timestamp
}

// Complete reports whether some path reaches a bitcoin attestation, and the
// lowest block height found.
func (t *Timestamp) Complete() (int, bool) {
	best, found := 0, false
	for _, a := range t.Attestations {
		if h, ok := a.Bitcoin(); ok && (!found || h < best) {
			best, found = h, true
		}
	}
	for _, b := range t.Ops {
		if h, ok := b.Next.Complete(); ok && (!found || h < best) {
			best, found = h, true
		}
	}
	return best, found
}

// pendings lists every node still waiting on a calendar.
func (t *Timestamp) pendings() []*Timestamp {
	var out []*Timestamp
	for _, a := range t.Attestations {
		if _, ok := a.Pending(); ok {
			out = append(out, t)
			break
		}
	}
	for _, b := range t.Ops {
		out = append(out, b.Next.pendings()...)
	}
	return out
}

// Merge folds another timestamp of the same message into this one.
func (t *Timestamp) Merge(o *Timestamp) error {
	if !bytes.Equal(t.Msg, o.Msg) {
		return errors.New("ots: cannot merge timestamps of different messages")
	}
	for _, a := range o.Attestations {
		t.addAttestation(a)
	}
	for _, ob := range o.Ops {
		merged := false
		for _, tb := range t.Ops {
			if tb.Op.equal(ob.Op) {
				if err := tb.Next.Merge(ob.Next); err != nil {
					return err
				}
				merged = true
				break
			}
		}
		if !merged {
			t.Ops = append(t.Ops, ob)
		}
	}
	return nil
}

func (t *Timestamp) addAttestation(a Attestation) {
	for _, have := range t.Attestations {
		if have.equal(a) {
			return
		}
	}
	t.Attestations = append(t.Attestations, a)
}

// dropPending removes the pending attestations of this node, once the
// calendar they name has been followed to something better.
func (t *Timestamp) dropPending() {
	kept := t.Attestations[:0]
	for _, a := range t.Attestations {
		if _, ok := a.Pending(); !ok {
			kept = append(kept, a)
		}
	}
	t.Attestations = kept
}

// Serialize writes the timestamp in the reference format.
func (t *Timestamp) Serialize() ([]byte, error) {
	var w writer
	if err := t.serialize(&w); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

func (t *Timestamp) serialize(w *writer) error {
	if len(t.Attestations) == 0 && len(t.Ops) == 0 {
		return errors.New("ots: an empty timestamp cannot be serialized")
	}
	atts := append([]Attestation{}, t.Attestations...)
	sort.Slice(atts, func(i, j int) bool { return atts[i].less(atts[j]) })
	ops := append([]Branch{}, t.Ops...)
	sort.Slice(ops, func(i, j int) bool { return ops[i].Op.less(ops[j].Op) })

	if len(ops) == 0 {
		for _, a := range atts[:len(atts)-1] {
			w.bytes([]byte{0xff, 0x00})
			writeAttestation(w, a)
		}
		w.bytes([]byte{0x00})
		writeAttestation(w, atts[len(atts)-1])
		return nil
	}
	for _, a := range atts {
		w.bytes([]byte{0xff, 0x00})
		writeAttestation(w, a)
	}
	for i, b := range ops {
		if i < len(ops)-1 {
			w.bytes([]byte{0xff})
		}
		writeOp(w, b.Op)
		if err := b.Next.serialize(w); err != nil {
			return err
		}
	}
	return nil
}

func writeOp(w *writer, o Op) {
	w.bytes([]byte{o.Tag})
	if o.Tag == opAppend || o.Tag == opPrepend {
		w.varbytes(o.Arg)
	}
}

func writeAttestation(w *writer, a Attestation) {
	w.bytes(a.Tag[:])
	w.varbytes(a.Payload)
}

// ParseTimestamp reads a serialized timestamp of msg — the shape a calendar
// returns from POST /digest and GET /timestamp/{commitment}.
func ParseTimestamp(b []byte, msg []byte) (*Timestamp, error) {
	r := &reader{b: b}
	t, err := parseTimestamp(r, msg, maxDepth)
	if err != nil {
		return nil, err
	}
	if r.rest() != 0 {
		return nil, errors.New("ots: trailing bytes after timestamp")
	}
	return t, nil
}

func parseTimestamp(r *reader, msg []byte, depth int) (*Timestamp, error) {
	if depth == 0 {
		return nil, errors.New("ots: timestamp nested too deep")
	}
	t := &Timestamp{Msg: msg}
	one := func(tag byte) error {
		if tag == 0x00 {
			a, err := readAttestation(r)
			if err != nil {
				return err
			}
			t.addAttestation(a)
			return nil
		}
		op, err := readOp(r, tag)
		if err != nil {
			return err
		}
		// An op we cannot compute leaves the message below unknown; the
		// structure is still kept so the proof round-trips intact.
		next, _ := op.Apply(msg)
		sub, err := parseTimestamp(r, next, depth-1)
		if err != nil {
			return err
		}
		t.Ops = append(t.Ops, Branch{Op: op, Next: sub})
		return nil
	}
	tag, err := r.byte()
	if err != nil {
		return nil, err
	}
	for tag == 0xff {
		inner, err := r.byte()
		if err != nil {
			return nil, err
		}
		if err := one(inner); err != nil {
			return nil, err
		}
		if tag, err = r.byte(); err != nil {
			return nil, err
		}
	}
	if err := one(tag); err != nil {
		return nil, err
	}
	return t, nil
}

func readOp(r *reader, tag byte) (Op, error) {
	switch tag {
	case opAppend, opPrepend:
		arg, err := r.varbytes(maxOpArg)
		if err != nil {
			return Op{}, err
		}
		if len(arg) == 0 {
			return Op{}, errors.New("ots: empty binary op argument")
		}
		return Op{Tag: tag, Arg: arg}, nil
	case opSHA1, opRIPEMD160, opSHA256, opKeccak256, opReverse, opHexlify:
		return Op{Tag: tag}, nil
	}
	return Op{}, fmt.Errorf("ots: unknown op tag %#x", tag)
}

func readAttestation(r *reader) (Attestation, error) {
	var a Attestation
	tag, err := r.take(8)
	if err != nil {
		return a, err
	}
	copy(a.Tag[:], tag)
	a.Payload, err = r.varbytes(maxPayload)
	return a, err
}

// SerializeFile writes a detached .ots file for a SHA-256 digest.
func SerializeFile(digest []byte, t *Timestamp) ([]byte, error) {
	if len(digest) != sha256.Size {
		return nil, errors.New("ots: file digest must be SHA-256")
	}
	if !bytes.Equal(digest, t.Msg) {
		return nil, errors.New("ots: timestamp is not of this digest")
	}
	var w writer
	w.bytes(otsMagic)
	w.varuint(otsMajorVersion)
	w.bytes([]byte{opSHA256})
	w.bytes(digest)
	if err := t.serialize(&w); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

// ParseFile reads a detached .ots file back.
func ParseFile(b []byte) ([]byte, *Timestamp, error) {
	r := &reader{b: b}
	magic, err := r.take(len(otsMagic))
	if err != nil || !bytes.Equal(magic, otsMagic) {
		return nil, nil, errors.New("ots: not an OpenTimestamps proof")
	}
	v, err := r.varuint()
	if err != nil || v != otsMajorVersion {
		return nil, nil, errors.New("ots: unsupported proof version")
	}
	tag, err := r.byte()
	if err != nil || tag != opSHA256 {
		return nil, nil, errors.New("ots: proof is not over a SHA-256 digest")
	}
	digest, err := r.take(sha256.Size)
	if err != nil {
		return nil, nil, err
	}
	t, err := parseTimestamp(r, digest, maxDepth)
	if err != nil {
		return nil, nil, err
	}
	if r.rest() != 0 {
		return nil, nil, errors.New("ots: trailing bytes after proof")
	}
	return digest, t, nil
}

// reader and writer speak the format's varints and length-prefixed bytes.
type reader struct {
	b []byte
	i int
}

func (r *reader) rest() int { return len(r.b) - r.i }

func (r *reader) byte() (byte, error) {
	if r.i >= len(r.b) {
		return 0, errors.New("ots: truncated")
	}
	b := r.b[r.i]
	r.i++
	return b, nil
}

func (r *reader) take(n int) ([]byte, error) {
	if n < 0 || r.i+n > len(r.b) {
		return nil, errors.New("ots: truncated")
	}
	out := r.b[r.i : r.i+n]
	r.i += n
	return out, nil
}

func (r *reader) varuint() (uint64, error) {
	var v uint64
	for shift := 0; shift < 64; shift += 7 {
		b, err := r.byte()
		if err != nil {
			return 0, err
		}
		v |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return v, nil
		}
	}
	return 0, errors.New("ots: varint too long")
}

func (r *reader) varbytes(max int) ([]byte, error) {
	n, err := r.varuint()
	if err != nil {
		return nil, err
	}
	if n > uint64(max) {
		return nil, errors.New("ots: length prefix exceeds limit")
	}
	return r.take(int(n))
}

type writer struct{ bytes.Buffer }

func (w *writer) bytes(b []byte) { w.Write(b) }

func (w *writer) varuint(v uint64) {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], v)
	w.Write(buf[:n])
}

func (w *writer) varbytes(b []byte) {
	w.varuint(uint64(len(b)))
	w.Write(b)
}
