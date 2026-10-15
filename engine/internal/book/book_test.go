package book

import (
	"testing"

	"dyanis-chess/engine/internal/board"
	"dyanis-chess/engine/internal/movegen"
)

// encode mirrors Polyglot's 16-bit move packing, for building synthetic
// test entries without hand-computing bit patterns.
func encode(fromFile, fromRank, toFile, toRank int, promo uint16) uint16 {
	return uint16(toFile) | uint16(toRank)<<3 | uint16(fromFile)<<6 | uint16(fromRank)<<9 | promo<<12
}

func TestDecodeRawQuietMove(t *testing.T) {
	raw := encode(4, 1, 4, 3, 0) // e2 -> e4, no promotion
	from, to, promo := decodeRaw(raw)

	wantFrom, _ := board.ParseSquare("e2")
	wantTo, _ := board.ParseSquare("e4")
	if from != wantFrom || to != wantTo {
		t.Errorf("decodeRaw = (%v, %v), want (%v, %v)", from, to, wantFrom, wantTo)
	}
	if promo != board.NoType {
		t.Errorf("promotion = %v, want NoType", promo)
	}
}

func TestDecodeRawPromotion(t *testing.T) {
	raw := encode(6, 6, 6, 7, 4) // g7 -> g8, promo bits = 4 (queen)
	_, _, promo := decodeRaw(raw)
	if promo != board.Queen {
		t.Errorf("promotion = %v, want Queen", promo)
	}
}

func TestNormalizeCastling(t *testing.T) {
	cases := []struct {
		name         string
		fromAlg      string
		toAlg        string
		wantToAlg    string
		isCastleCase bool
	}{
		{"white kingside", "e1", "h1", "g1", true},
		{"white queenside", "e1", "a1", "c1", true},
		{"black kingside", "e8", "h8", "g8", true},
		{"black queenside", "e8", "a8", "c8", true},
		{"ordinary move untouched", "e2", "e4", "e4", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			from, _ := board.ParseSquare(c.fromAlg)
			to, _ := board.ParseSquare(c.toAlg)
			wantTo, _ := board.ParseSquare(c.wantToAlg)

			got := normalizeCastling(from, to)
			if got != wantTo {
				t.Errorf("normalizeCastling(%s,%s) = %v, want %v", c.fromAlg, c.toAlg, got, wantTo)
			}
		})
	}
}

func TestDecodeMoveQuiet(t *testing.T) {
	b := board.NewInitialBoard()
	legal := movegen.GenerateLegalMoves(b)

	entry := Entry{Move: encode(4, 1, 4, 3, 0)} // e2e4
	m, ok := DecodeMove(entry, legal)
	if !ok {
		t.Fatal("DecodeMove: expected a match for e2e4 in the starting position")
	}
	if m.String() != "e2e4" {
		t.Errorf("decoded move = %q, want %q", m.String(), "e2e4")
	}
}

func TestDecodeMoveCastling(t *testing.T) {
	// White king on e1, rook on h1, nothing in the way, no checks:
	// kingside castling is legal.
	b, err := board.FromFEN("4k3/8/8/8/8/8/8/4K2R w K - 0 1")
	if err != nil {
		t.Fatalf("FromFEN: %v", err)
	}
	legal := movegen.GenerateLegalMoves(b)

	// Polyglot encodes this as "king takes own rook": e1h1.
	entry := Entry{Move: encode(4, 0, 7, 0, 0)}
	m, ok := DecodeMove(entry, legal)
	if !ok {
		t.Fatal("DecodeMove: expected a match for the e1h1-encoded castling move")
	}
	if m.Flag != board.CastleKingside {
		t.Errorf("decoded move flag = %v, want CastleKingside", m.Flag)
	}
	if m.String() != "e1g1" {
		t.Errorf("decoded move = %q, want %q", m.String(), "e1g1")
	}
}

func TestDecodeMoveNoMatch(t *testing.T) {
	b := board.NewInitialBoard()
	legal := movegen.GenerateLegalMoves(b)

	// e2e5 isn't a legal move from the starting position.
	entry := Entry{Move: encode(4, 1, 4, 4, 0)}
	if _, ok := DecodeMove(entry, legal); ok {
		t.Error("DecodeMove: expected no match for an illegal move")
	}
}

// linearLookup is the straightforward reference Lookup is checked against.
func linearLookup(entries []Entry, key uint64) []Entry {
	var out []Entry
	for _, e := range entries {
		if e.Key == key {
			out = append(out, e)
		}
	}
	return out
}

func TestLookupUnsortedInputKeepsPerKeyOrder(t *testing.T) {
	in := []Entry{
		{Key: 9, Move: 1}, {Key: 3, Move: 2}, {Key: 9, Move: 3},
		{Key: 5, Move: 4}, {Key: 3, Move: 5}, {Key: 9, Move: 6},
	}
	bk := New(in)
	for _, key := range []uint64{3, 5, 9, 0, 4, 100} {
		got, want := bk.Lookup(key), linearLookup(in, key)
		if len(got) != len(want) {
			t.Fatalf("key %d: got %d entries, want %d", key, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("key %d entry %d: got %+v, want %+v (per-key order must be kept)", key, i, got[i], want[i])
			}
		}
	}
	if in[0].Key != 9 {
		t.Error("New must not reorder the caller's slice")
	}
}

// Every shipped book: binary-search Lookup must agree with a linear scan
// for the start position and for keys that are present and absent.
func TestLookupMatchesLinearScanOnRealBooks(t *testing.T) {
	for _, name := range []string{"gm2001.bin", "performance.bin", "komodo.bin"} {
		bk, err := Load("../../assets/" + name)
		if err != nil {
			t.Skipf("%s not available: %v", name, err)
		}
		keys := []uint64{board.NewInitialBoard().Hash(), 0, ^uint64(0)}
		step := len(bk.entries)/50 + 1
		for i := 0; i < len(bk.entries); i += step {
			keys = append(keys, bk.entries[i].Key, bk.entries[i].Key+1)
		}
		for _, k := range keys {
			got, want := bk.Lookup(k), linearLookup(bk.entries, k)
			if len(got) != len(want) {
				t.Fatalf("%s key %#x: got %d, want %d", name, k, len(got), len(want))
			}
		}
		if len(bk.Lookup(board.NewInitialBoard().Hash())) == 0 {
			t.Errorf("%s: no entries for the start position", name)
		}
	}
}
