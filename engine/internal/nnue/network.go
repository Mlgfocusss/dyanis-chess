// Package nnue evaluates positions with a small efficiently updatable
// neural network: 768 inputs per perspective (piece type × color relative
// to the perspective × square, flipped for Black), one hidden layer kept
// incrementally as an accumulator, and a single output.
//
//	hidden[p] = B1 + Σ W1[feature]       for each perspective p (Accumulator)
//	out       = W2 · [act(hidden[stm]), act(hidden[other])] + B2
//
// Weights are integers (quantized by the trainer): W1/B1 at scale QA,
// W2 at scale QB, B2 at QA·QB, and the result is scaled to centipawns by
// Scale/(QA·QB). The file format and the feature layout are specified in
// NNUE.md; the trainer must produce exactly what Parse reads.
package nnue

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
)

// Inputs is the number of input features per perspective and king bucket.
const Inputs = 768

// FormatVersion is the newest version of the weights file. Marshal writes
// it only for a network with king buckets or mirroring, and version 2
// otherwise; Parse reads versions 1 (a single output bucket), 2 and 3.
const FormatVersion = 3

const formatV2 = 2

const formatV1 = 1

const magic = "DYNN"

// Header sizes: magic + seven uint32/int32 fields in version 1, plus the
// bucket count in version 2.
const (
	headerSizeV1 = 4 + 7*4
	headerSizeV2 = headerSizeV1 + 4
	// Version 3 adds the king bucket count, the mirror flag and the 64-entry
	// king bucket map.
	headerSize = headerSizeV2 + 4 + 4 + 64
)

// maxBuckets bounds the number of output buckets a file may declare.
const maxBuckets = 32

// maxKingBuckets bounds the number of king buckets a file may declare.
const maxKingBuckets = 32

// maxHidden bounds the hidden layer size a file may declare.
const maxHidden = 4096

// Activation is the function applied to the accumulator before the
// output layer.
type Activation uint32

const (
	// CReLU clamps to [0, QA].
	CReLU Activation = 0
	// SCReLU clamps to [0, QA] and squares.
	SCReLU Activation = 1
)

func (a Activation) String() string {
	switch a {
	case CReLU:
		return "CReLU"
	case SCReLU:
		return "SCReLU"
	}
	return fmt.Sprintf("Activation(%d)", uint32(a))
}

// Network holds quantized weights.
type Network struct {
	Hidden     int
	Activation Activation
	QA, QB     int32
	Scale      int32

	// KingBuckets is the number of input feature sets; the perspective's own
	// king square picks one (KingMap). Mirror folds the board left-right so
	// that the king is always on files a–d. 1 and false give the plain 768
	// inputs.
	KingBuckets int
	Mirror      bool
	// KingMap gives the bucket of the king square as the perspective sees it
	// (rank-flipped for Black, file-flipped when mirroring and the king is
	// on e–h, so only a–d entries are used with Mirror).
	KingMap [64]uint8

	W1 []int16 // KingBuckets × Inputs × Hidden, feature-major: feature f is W1[f*Hidden:(f+1)*Hidden]
	B1 []int16 // Hidden
	// Buckets is the number of output layers; the position's piece count
	// picks one (see Bucket). 1 means a single output layer.
	Buckets int
	W2      []int16 // Buckets × 2·Hidden; per bucket the side to move's half first, then the other side's
	B2      []int32 // Buckets
}

// Bucket returns the output layer used for a position with the given
// number of pieces on the board (kings included): (pieces − 2) / 4, so 2–5
// pieces is bucket 0 and 30–32 is bucket 7 of 8, capped at the last bucket.
func (n *Network) Bucket(pieces int) int {
	return min(max(pieces-2, 0)/4, n.Buckets-1)
}

// New returns a network with a single output layer and all weights zero.
func New(hidden int, act Activation, qa, qb, scale int32) (*Network, error) {
	return NewBucketed(hidden, 1, act, qa, qb, scale)
}

// NewBucketed returns a network with the given number of output layers and
// all weights zero.
func NewBucketed(hidden, buckets int, act Activation, qa, qb, scale int32) (*Network, error) {
	return NewKinged(hidden, buckets, 1, false, [64]uint8{}, act, qa, qb, scale)
}

// NewKinged returns a network with output buckets and king-bucketed input
// features (see KingBuckets), all weights zero.
func NewKinged(hidden, buckets, kingBuckets int, mirror bool, kingMap [64]uint8, act Activation, qa, qb, scale int32) (*Network, error) {
	h, k := max(hidden, 0), max(buckets, 0)
	n := &Network{
		Hidden: hidden, Activation: act, QA: qa, QB: qb, Scale: scale, Buckets: buckets,
		KingBuckets: kingBuckets, Mirror: mirror, KingMap: kingMap,
		W1: make([]int16, Inputs*min(max(kingBuckets, 0), maxKingBuckets)*h),
		B1: make([]int16, h),
		W2: make([]int16, 2*h*k),
		B2: make([]int32, k),
	}
	return n, n.validate()
}

func (n *Network) validate() error {
	switch {
	case n.Hidden < 1 || n.Hidden > maxHidden:
		return fmt.Errorf("nnue: hidden size %d outside [1, %d]", n.Hidden, maxHidden)
	case n.Activation != CReLU && n.Activation != SCReLU:
		return fmt.Errorf("nnue: unknown activation %d", uint32(n.Activation))
	case n.QA < 1 || n.QA > 255 || n.QB < 1 || n.QB > 32767:
		return fmt.Errorf("nnue: quantization QA=%d QB=%d outside [1, 255] / [1, 32767]", n.QA, n.QB)
	case n.Scale < 1:
		return fmt.Errorf("nnue: scale %d must be positive", n.Scale)
	case n.Buckets < 1 || n.Buckets > maxBuckets:
		return fmt.Errorf("nnue: %d output buckets outside [1, %d]", n.Buckets, maxBuckets)
	case n.KingBuckets < 1 || n.KingBuckets > maxKingBuckets:
		return fmt.Errorf("nnue: %d king buckets outside [1, %d]", n.KingBuckets, maxKingBuckets)
	case len(n.W1) != Inputs*n.KingBuckets*n.Hidden || len(n.B1) != n.Hidden || len(n.W2) != 2*n.Hidden*n.Buckets || len(n.B2) != n.Buckets:
		return errors.New("nnue: weight slices do not match the hidden size")
	}
	for sq, b := range n.KingMap {
		if int(b) >= n.KingBuckets {
			return fmt.Errorf("nnue: king square %d is in bucket %d of %d", sq, b, n.KingBuckets)
		}
	}
	for i, w := range n.W2 {
		if w < -MaxOutputWeight || w > MaxOutputWeight {
			return fmt.Errorf("nnue: output weight %d is %d, outside ±%d", i, w, MaxOutputWeight)
		}
	}
	return nil
}

// MaxOutputWeight bounds |W2|. It keeps every product in Evaluate within
// int32 (an SCReLU term is at most QA²·127 ≈ 8.3·10⁶ at QA = 255), and
// act·w within int16 for SIMD. With QB = 64 it means float weights within
// ±1.98, which trainers clip to anyway.
const MaxOutputWeight = 127

// Marshal encodes n in the weights file format (see NNUE.md).
func (n *Network) Marshal() ([]byte, error) {
	if err := n.validate(); err != nil {
		return nil, err
	}
	version, hdr := uint32(formatV2), headerSizeV2
	if n.KingBuckets > 1 || n.Mirror {
		version, hdr = FormatVersion, headerSize
	}
	size := hdr + 2*(len(n.W1)+len(n.B1)+len(n.W2)) + 4*len(n.B2) + 4
	out := make([]byte, 0, size)
	out = append(out, magic...)
	for _, v := range []uint32{version, Inputs, uint32(n.Hidden), uint32(n.Activation), uint32(n.QA), uint32(n.QB), uint32(n.Scale), uint32(n.Buckets)} {
		out = binary.LittleEndian.AppendUint32(out, v)
	}
	if version == FormatVersion {
		mirror := uint32(0)
		if n.Mirror {
			mirror = 1
		}
		out = binary.LittleEndian.AppendUint32(out, uint32(n.KingBuckets))
		out = binary.LittleEndian.AppendUint32(out, mirror)
		out = append(out, n.KingMap[:]...)
	}
	for _, s := range [][]int16{n.W1, n.B1, n.W2} {
		for _, v := range s {
			out = binary.LittleEndian.AppendUint16(out, uint16(v))
		}
	}
	for _, v := range n.B2 {
		out = binary.LittleEndian.AppendUint32(out, uint32(v))
	}
	return binary.LittleEndian.AppendUint32(out, crc32.ChecksumIEEE(out)), nil
}

// Load reads and parses the weights file at path.
func Load(path string) (*Network, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	n, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return n, nil
}

// Parse decodes a weights file (version 1, 2 or 3). Anything unexpected —
// wrong magic, version, input count, sizes, a bad checksum or trailing
// bytes — is an error: a network that loads must be exactly the one that
// was exported.
func Parse(data []byte) (*Network, error) {
	if len(data) < headerSizeV1+4 {
		return nil, fmt.Errorf("nnue: file of %d bytes is too short", len(data))
	}
	if string(data[:4]) != magic {
		return nil, fmt.Errorf("nnue: bad magic %q", data[:4])
	}
	body, sum := data[:len(data)-4], binary.LittleEndian.Uint32(data[len(data)-4:])
	if crc32.ChecksumIEEE(body) != sum {
		return nil, errors.New("nnue: checksum mismatch")
	}
	u32 := func(i int) uint32 { return binary.LittleEndian.Uint32(data[4+4*i:]) }
	hdr, buckets := headerSizeV1, uint32(1)
	kingBuckets, mirror := uint32(1), false
	var kingMap [64]uint8
	switch v := u32(0); v {
	case formatV1:
	case formatV2:
		if len(data) < headerSizeV2+4 {
			return nil, fmt.Errorf("nnue: file of %d bytes is too short", len(data))
		}
		hdr, buckets = headerSizeV2, u32(7)
	case FormatVersion:
		if len(data) < headerSize+4 {
			return nil, fmt.Errorf("nnue: file of %d bytes is too short", len(data))
		}
		hdr, buckets = headerSize, u32(7)
		kingBuckets = u32(8)
		switch u32(9) {
		case 0:
		case 1:
			mirror = true
		default:
			return nil, fmt.Errorf("nnue: mirror flag %d", u32(9))
		}
		copy(kingMap[:], data[4+4*10:])
	default:
		return nil, fmt.Errorf("nnue: format version %d, this build reads %d to %d", v, formatV1, FormatVersion)
	}
	if in := u32(1); in != Inputs {
		return nil, fmt.Errorf("nnue: %d inputs, this build has %d", in, Inputs)
	}
	hidden := u32(2)
	if hidden < 1 || hidden > maxHidden {
		return nil, fmt.Errorf("nnue: hidden size %d outside [1, %d]", hidden, maxHidden)
	}
	if buckets < 1 || buckets > maxBuckets {
		return nil, fmt.Errorf("nnue: %d output buckets outside [1, %d]", buckets, maxBuckets)
	}
	if kingBuckets < 1 || kingBuckets > maxKingBuckets {
		return nil, fmt.Errorf("nnue: %d king buckets outside [1, %d]", kingBuckets, maxKingBuckets)
	}
	n, err := NewKinged(int(hidden), int(buckets), int(kingBuckets), mirror, kingMap, Activation(u32(3)), int32(u32(4)), int32(u32(5)), int32(u32(6)))
	if err != nil {
		return nil, err
	}
	want := hdr + 2*(len(n.W1)+len(n.B1)+len(n.W2)) + 4*len(n.B2)
	if len(body) != want {
		return nil, fmt.Errorf("nnue: %d bytes of weights for hidden size %d, %d output and %d king buckets, want %d", len(body), hidden, buckets, kingBuckets, want)
	}
	off := hdr
	for _, s := range [][]int16{n.W1, n.B1, n.W2} {
		for i := range s {
			s[i] = int16(binary.LittleEndian.Uint16(data[off:]))
			off += 2
		}
	}
	for i := range n.B2 {
		n.B2[i] = int32(binary.LittleEndian.Uint32(data[off:]))
		off += 4
	}
	return n, nil
}
