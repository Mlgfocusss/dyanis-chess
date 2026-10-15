package nnue

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"math/rand"
	"strings"
	"testing"

	"dyanis-chess/engine/internal/board"
	"dyanis-chess/engine/internal/eval"
	"dyanis-chess/engine/internal/movegen"
)

// randomNetwork fills a network with small random weights: |W1| ≤ 64 keeps
// a 32-piece accumulator far from int16 overflow, as a trained (clipped)
// network would be.
func randomNetwork(t testing.TB, hidden int, act Activation, seed int64) *Network {
	t.Helper()
	n, err := NewBucketed(hidden, 8, act, 255, 64, 400)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(seed))
	for _, s := range [][]int16{n.W1, n.B1, n.W2} {
		for i := range s {
			s[i] = int16(rng.Intn(129) - 64)
		}
	}
	for i := range n.B2 {
		n.B2[i] = int32(rng.Intn(20001) - 10000)
	}
	return n
}

var testFENs = []string{
	board.StartFEN,
	"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", // Kiwipete: castling, promotions, ep
	"rnbqkbnr/ppp1p1pp/8/3pPp2/8/8/PPPP1PPP/RNBQKBNR w KQkq f6 0 3",        // en passant for White
	"rnbqkbnr/pppp1ppp/8/8/3Pp3/8/PPP1PPPP/RNBQKBNR b KQkq d3 0 2",         // en passant for Black
	"n1n5/PPPk4/8/8/8/8/4Kppp/5N1N b - - 0 1",                              // promotions with captures
	"r3k2r/8/8/8/8/8/8/R3K2R b KQkq - 0 1",                                 // castling for Black
	"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",                            // ep pins, endgame
}

func TestMarshalParseRoundTrip(t *testing.T) {
	for _, act := range []Activation{CReLU, SCReLU} {
		n := randomNetwork(t, 32, act, 1)
		data, err := n.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		got, err := Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		again, _ := got.Marshal()
		if !bytes.Equal(data, again) {
			t.Fatalf("%v: round trip changed the file", act)
		}
	}
}

func TestParseRejectsBadFiles(t *testing.T) {
	good, err := randomNetwork(t, 16, CReLU, 2).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(good); err != nil {
		t.Fatalf("good file rejected: %v", err)
	}
	corrupt := func(f func([]byte) []byte) []byte {
		return f(append([]byte(nil), good...))
	}
	cases := map[string][]byte{
		"empty":     nil,
		"truncated": good[:len(good)/2],
		"magic":     corrupt(func(d []byte) []byte { d[0] = 'X'; return d }),
		"weight":    corrupt(func(d []byte) []byte { d[headerSizeV2+10]++; return d }),
		"trailing":  append(append([]byte(nil), good...), 0),
	}
	for name, data := range cases {
		if _, err := Parse(data); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A header change with a recomputed checksum must still be caught by
	// the field checks, not only by the checksum.
	for name, off := range map[string]int{"version": 4, "inputs": 8, "hidden": 12, "activation": 16, "buckets": 32} {
		data := append([]byte(nil), good...)
		data[off] += 7
		body := data[:len(data)-4]
		binary.LittleEndian.PutUint32(data[len(data)-4:], crc32.ChecksumIEEE(body))
		if _, err := Parse(data); err == nil {
			t.Errorf("%s changed: accepted", name)
		}
	}
}

// Every node of a few perft trees and of random games: the accumulator
// built move by move must equal one computed from scratch, for both
// perspectives and both activations.
func TestUpdateMatchesRefresh(t *testing.T) {
	for _, act := range []Activation{CReLU, SCReLU} {
		n := randomNetwork(t, 64, act, 3)
		checked := 0
		var walk func(b *board.Board, acc *Accumulator, depth int)
		walk = func(b *board.Board, acc *Accumulator, depth int) {
			if depth == 0 {
				return
			}
			for _, m := range movegen.GenerateLegalMoves(b) {
				u := b.MakeMove(m)
				child, fresh := n.NewAccumulator(), n.NewAccumulator()
				n.Update(child, acc, m, u, snapOf(n, b, m, u))
				n.Refresh(fresh, b)
				if !child.Equal(fresh) {
					b.UnmakeMove(m, u)
					t.Fatalf("%v: Update differs from Refresh after %v in %s", act, m, b.ToFEN())
				}
				checked++
				walk(b, child, depth-1)
				b.UnmakeMove(m, u)
			}
		}
		for _, fen := range testFENs {
			b, err := board.FromFEN(fen)
			if err != nil {
				t.Fatal(err)
			}
			acc := n.NewAccumulator()
			n.Refresh(acc, b)
			walk(b, acc, 3)
		}

		rng := rand.New(rand.NewSource(4))
		for g := 0; g < 100; g++ {
			b := board.NewInitialBoard()
			acc := n.NewAccumulator()
			n.Refresh(acc, b)
			for ply := 0; ply < 200; ply++ {
				legal := movegen.GenerateLegalMoves(b)
				if len(legal) == 0 {
					break
				}
				m := legal[rng.Intn(len(legal))]
				u := b.MakeMove(m)
				child, fresh := n.NewAccumulator(), n.NewAccumulator()
				n.Update(child, acc, m, u, snapOf(n, b, m, u))
				n.Refresh(fresh, b)
				if !child.Equal(fresh) {
					t.Fatalf("%v: random game %d: Update differs from Refresh after %v in %s", act, g, m, b.ToFEN())
				}
				acc = child
				checked++
			}
		}
		t.Logf("%v: %d positions checked", act, checked)
	}
}

// referenceEval is the network written as plainly as possible: scan the
// 64 squares, sum the hidden layer in int64, apply the activation, no
// accumulator. Evaluate must agree with it exactly.
func referenceEval(n *Network, b *board.Board) int {
	var hidden [2][]int64
	for persp := 0; persp < 2; persp++ {
		hidden[persp] = make([]int64, n.Hidden)
		for i := range hidden[persp] {
			hidden[persp][i] = int64(n.B1[i])
		}
		for sq := 0; sq < 64; sq++ {
			p := b.PieceAt(board.Square(sq))
			if p.IsNone() {
				continue
			}
			own := (p.Color() == board.White) == (persp == 0)
			rel, s := 1, sq
			if own {
				rel = 0
			}
			if persp == 1 {
				s = (7-sq/8)*8 + sq%8
			}
			f := rel*384 + (int(p.Type())-1)*64 + s
			for i := range hidden[persp] {
				hidden[persp][i] += int64(n.W1[f*n.Hidden+i])
			}
		}
	}
	stm := 0
	if b.SideToMove == board.Black {
		stm = 1
	}
	pieces := 0
	for sq := 0; sq < 64; sq++ {
		if !b.PieceAt(board.Square(sq)).IsNone() {
			pieces++
		}
	}
	bucket := (pieces - 2) / 4
	if bucket > n.Buckets-1 {
		bucket = n.Buckets - 1
	}
	qa := int64(n.QA)
	var sum int64
	for half, persp := range []int{stm, 1 - stm} {
		for i, x := range hidden[persp] {
			c := min(max(x, 0), qa)
			if n.Activation == SCReLU {
				c *= c
			}
			sum += c * int64(n.W2[bucket*2*n.Hidden+half*n.Hidden+i])
		}
	}
	if n.Activation == SCReLU {
		sum /= qa
	}
	sum += int64(n.B2[bucket])
	return int(sum * int64(n.Scale) / (qa * int64(n.QB)))
}

// mirrorFEN swaps the colors and flips the board vertically: the same
// position seen from the other side.
func mirrorFEN(t *testing.T, fen string) string {
	t.Helper()
	f := strings.Fields(fen)
	ranks := strings.Split(f[0], "/")
	for i, j := 0, len(ranks)-1; i < j; i, j = i+1, j-1 {
		ranks[i], ranks[j] = ranks[j], ranks[i]
	}
	swap := func(s string) string {
		return strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z':
				return r - 32
			case r >= 'A' && r <= 'Z':
				return r + 32
			}
			return r
		}, s)
	}
	side := "w"
	if f[1] == "w" {
		side = "b"
	}
	castle := "-"
	if f[2] != "-" {
		c := swap(f[2])
		castle = ""
		for _, r := range "KQkq" {
			if strings.ContainsRune(c, r) {
				castle += string(r)
			}
		}
	}
	ep := "-"
	if f[3] != "-" {
		ep = string(f[3][0]) + string(rune('1'+'8'-f[3][1]))
	}
	return strings.Join([]string{swap(strings.Join(ranks, "/")), side, castle, ep, f[4], f[5]}, " ")
}

func TestEvaluateMatchesReferenceAndMirror(t *testing.T) {
	for _, act := range []Activation{CReLU, SCReLU} {
		n := randomNetwork(t, 48, act, 5)
		rng := rand.New(rand.NewSource(6))
		positions := 0
		check := func(b *board.Board) {
			t.Helper()
			acc := n.NewAccumulator()
			n.Refresh(acc, b)
			got := n.Evaluate(acc, b.SideToMove, b.Occupied().Count())
			if want := referenceEval(n, b); got != want {
				t.Fatalf("%v: %s: Evaluate %d, reference %d", act, b.ToFEN(), got, want)
			}
			mb, err := board.FromFEN(mirrorFEN(t, b.ToFEN()))
			if err != nil {
				t.Fatal(err)
			}
			macc := n.NewAccumulator()
			n.Refresh(macc, mb)
			if m := n.Evaluate(macc, mb.SideToMove, mb.Occupied().Count()); m != got {
				t.Fatalf("%v: %s scores %d, its mirror %s scores %d", act, b.ToFEN(), got, mb.ToFEN(), m)
			}
			positions++
		}
		for _, fen := range testFENs {
			b, _ := board.FromFEN(fen)
			check(b)
		}
		for g := 0; g < 50; g++ {
			b := board.NewInitialBoard()
			for ply := 0; ply < 120; ply++ {
				legal := movegen.GenerateLegalMoves(b)
				if len(legal) == 0 {
					break
				}
				b.MakeMove(legal[rng.Intn(len(legal))])
				check(b)
			}
		}
		t.Logf("%v: %d positions", act, positions)
	}
}

func TestMirrorFEN(t *testing.T) {
	got := mirrorFEN(t, "rnbqkbnr/pppp1ppp/8/8/3Pp3/8/PPP1PPPP/RNBQKBNR b Kq d3 0 2")
	want := "rnbqkbnr/ppp1pppp/8/3pP3/8/8/PPPP1PPP/RNBQKBNR w Qk d6 0 2"
	if got != want {
		t.Fatalf("mirrorFEN:\n got %s\nwant %s", got, want)
	}
}

// --- speed, for comparison with the hand-written eval -------------------

func benchPosition(b *testing.B) (*board.Board, board.Move) {
	pos, err := board.FromFEN(testFENs[1])
	if err != nil {
		b.Fatal(err)
	}
	for _, m := range movegen.GenerateLegalMoves(pos) {
		if m.Flag == board.Quiet {
			return pos, m
		}
	}
	b.Fatal("no quiet move")
	return nil, board.Move{}
}

func BenchmarkUpdate256(b *testing.B) {
	n := randomNetwork(b, 256, SCReLU, 7)
	pos, m := benchPosition(b)
	src, dst := n.NewAccumulator(), n.NewAccumulator()
	n.Refresh(src, pos)
	u := pos.MakeMove(m)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n.Update(dst, src, m, u, snapOf(n, pos, m, u))
	}
}

func BenchmarkEvaluate256(b *testing.B) {
	for _, act := range []Activation{CReLU, SCReLU} {
		b.Run(act.String(), func(b *testing.B) {
			n := randomNetwork(b, 256, act, 7)
			pos, _ := benchPosition(b)
			acc := n.NewAccumulator()
			n.Refresh(acc, pos)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				n.Evaluate(acc, pos.SideToMove, pos.Occupied().Count())
			}
		})
	}
}

func BenchmarkHandEval(b *testing.B) {
	pos, _ := benchPosition(b)
	cache := eval.NewPawnCache()
	for i := 0; i < b.N; i++ {
		eval.Evaluate(pos, cache)
	}
}

func TestBucket(t *testing.T) {
	n := randomNetwork(t, 8, CReLU, 1)
	for pieces, want := range map[int]int{2: 0, 5: 0, 6: 1, 9: 1, 10: 2, 29: 6, 30: 7, 32: 7, 40: 7} {
		if got := n.Bucket(pieces); got != want {
			t.Errorf("Bucket(%d) = %d, want %d", pieces, got, want)
		}
	}
	one, err := New(8, CReLU, 255, 64, 400)
	if err != nil {
		t.Fatal(err)
	}
	if got := one.Bucket(32); got != 0 {
		t.Errorf("single-bucket network: Bucket(32) = %d, want 0", got)
	}
}

// A version 1 file (one output layer, no bucket field) must still load, as
// the single-bucket network it describes, and round-trip through version 2.
func TestParseVersion1(t *testing.T) {
	n, err := New(16, SCReLU, 255, 64, 400)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(3))
	for _, s := range [][]int16{n.W1, n.B1, n.W2} {
		for i := range s {
			s[i] = int16(rng.Intn(129) - 64)
		}
	}
	n.B2[0] = -4321
	v2, err := n.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	// v1 = the v2 bytes without the bucket field, version 1, new checksum.
	v1 := append([]byte(nil), v2[:headerSizeV1]...)
	v1 = append(v1, v2[headerSizeV2:len(v2)-4]...)
	binary.LittleEndian.PutUint32(v1[4:], formatV1)
	v1 = binary.LittleEndian.AppendUint32(v1, crc32.ChecksumIEEE(v1))
	got, err := Parse(v1)
	if err != nil {
		t.Fatalf("version 1 file rejected: %v", err)
	}
	if got.Buckets != 1 || got.B2[0] != -4321 {
		t.Fatalf("version 1: buckets %d, B2 %v", got.Buckets, got.B2)
	}
	again, err := got.Marshal()
	if err != nil || string(again) != string(v2) {
		t.Fatalf("version 1 file does not round-trip through version 2 (err %v)", err)
	}
}

// snapOf is the bitboard argument Update wants: the position after the move
// when the move needs a king refresh, nil otherwise.
func snapOf(n *Network, b *board.Board, m board.Move, u board.Undo) *[12]board.Bitboard {
	if !n.KingRefresh(m, u) {
		return nil
	}
	p := b.PieceBitboards()
	return &p
}

// kingedNetwork is a random network with 5 king buckets and mirroring; the
// map is chosen so that king moves cross buckets often.
func kingedNetwork(t testing.TB, hidden int, mirror bool, seed int64) *Network {
	t.Helper()
	var km [64]uint8
	for sq := range km {
		f, r := sq&7, sq>>3
		km[sq] = uint8((f/2 + r) % 5)
	}
	n, err := NewKinged(hidden, 8, 5, mirror, km, SCReLU, 255, 64, 400)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(seed))
	for _, s := range [][]int16{n.W1, n.B1, n.W2} {
		for i := range s {
			s[i] = int16(rng.Intn(129) - 64)
		}
	}
	return n
}

// With king buckets (and mirroring) Update must still equal Refresh at every
// node, including king moves that change bucket or cross the mirror line.
func TestKingBucketsUpdateMatchesRefresh(t *testing.T) {
	for _, mirror := range []bool{false, true} {
		n := kingedNetwork(t, 32, mirror, 5)
		refreshes, checked := 0, 0
		rng := rand.New(rand.NewSource(9))
		for g := 0; g < 300; g++ {
			b := board.NewInitialBoard()
			if g%3 == 1 {
				b, _ = board.FromFEN(testFENs[1+g%6])
			}
			acc := n.NewAccumulator()
			n.Refresh(acc, b)
			for ply := 0; ply < 200; ply++ {
				legal := movegen.GenerateLegalMoves(b)
				if len(legal) == 0 {
					break
				}
				m := legal[rng.Intn(len(legal))]
				u := b.MakeMove(m)
				if n.KingRefresh(m, u) {
					refreshes++
				}
				child, fresh := n.NewAccumulator(), n.NewAccumulator()
				n.Update(child, acc, m, u, snapOf(n, b, m, u))
				n.Refresh(fresh, b)
				if !child.Equal(fresh) {
					t.Fatalf("mirror=%v game %d: Update differs from Refresh after %v in %s", mirror, g, m, b.ToFEN())
				}
				acc = child
				checked++
			}
		}
		if refreshes == 0 {
			t.Fatal("no king move changed bucket: the test checks nothing")
		}
		t.Logf("mirror=%v: %d positions, %d king refreshes", mirror, checked, refreshes)
	}
}

func TestKingedMarshalRoundTrip(t *testing.T) {
	n := kingedNetwork(t, 16, true, 3)
	data, err := n.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := got.Marshal()
	if !bytes.Equal(data, again) || got.KingBuckets != 5 || !got.Mirror || got.KingMap != n.KingMap {
		t.Fatal("king-bucketed network does not round-trip")
	}
}
