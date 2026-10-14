package trainingdata

import (
	"math/rand"
	"testing"

	"dyanis-chess/internal/board"
	"dyanis-chess/internal/movegen"
)

func roundTrip(t *testing.T, b *board.Board, l Label) {
	t.Helper()
	var rec [RecordSize]byte
	if err := Encode(rec[:], b, l); err != nil {
		t.Fatalf("%s: encode: %v", b.ToFEN(), err)
	}
	fen, got, err := DecodeFEN(rec[:])
	if err != nil {
		t.Fatalf("%s: decode: %v", b.ToFEN(), err)
	}
	if fen != b.ToFEN() {
		t.Fatalf("FEN round trip:\n got %s\nwant %s", fen, b.ToFEN())
	}
	want := l
	if want.Mate {
		want.Score = MateScore
		if l.Score < 0 {
			want.Score = -MateScore
		}
	} else {
		want.Score = min(max(want.Score, -MaxScore), MaxScore)
	}
	if got != want {
		t.Fatalf("%s: label round trip: got %+v want %+v", fen, got, want)
	}
	if _, _, err := Decode(rec[:]); err != nil {
		t.Fatalf("%s: Decode: %v", fen, err)
	}
}

func TestRoundTripSpecialPositions(t *testing.T) {
	fens := []string{
		board.StartFEN, // 32 pieces, all castling rights
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", // Kiwipete
		"rnbqkbnr/ppp1p1pp/8/3pPp2/8/8/PPPP1PPP/RNBQKBNR w KQkq f6 0 3",        // en passant, white to move
		"rnbqkbnr/pppp1ppp/8/8/3Pp3/8/PPP1PPPP/RNBQKBNR b Kq d3 0 2",           // en passant, black to move, partial castling
		"8/8/8/8/8/8/8/K6k b - - 99 140",                                       // bare kings, high clocks
		"4k3/1P6/8/8/8/8/6p1/4K3 w - - 0 60",                                   // pawns about to promote
		"r3k3/8/8/8/8/8/8/4K2R w Kq - 3 30",                                    // one right each
		"7k/8/8/8/8/8/8/QQQQKQQQ b - - 0 1",                                    // many queens, h-file king
	}
	labels := []Label{
		{Score: 0, Result: Draw},
		{Score: 123, InCheck: true, Result: WhiteWin},
		{Score: -4567, NoisyBest: true, Result: BlackWin},
		{Score: 900000, Mate: true, Result: WhiteWin},
		{Score: -899990, Mate: true, Result: BlackWin},
		{Score: 50000, Result: Draw},  // clamped to MaxScore
		{Score: -50000, Result: Draw}, // clamped to -MaxScore
		{Score: 31999, InCheck: true, NoisyBest: true, Result: WhiteWin},
	}
	for i, fen := range fens {
		b, err := board.FromFEN(fen)
		if err != nil {
			t.Fatal(err)
		}
		roundTrip(t, b, labels[i%len(labels)])
	}
}

// Every position of a few hundred random games (castling, en passant,
// promotions and captures all show up) must survive the round trip.
func TestRoundTripRandomGames(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	positions := 0
	for g := 0; g < 300; g++ {
		b := board.NewInitialBoard()
		for ply := 0; ply < 300; ply++ {
			legal := movegen.GenerateLegalMoves(b)
			if len(legal) == 0 {
				break
			}
			l := Label{
				Score:     rng.Intn(4001) - 2000,
				Mate:      rng.Intn(20) == 0,
				InCheck:   movegen.InCheck(b),
				NoisyBest: rng.Intn(2) == 0,
				Result:    uint8(rng.Intn(3)),
			}
			roundTrip(t, b, l)
			positions++
			b.MakeMove(legal[rng.Intn(len(legal))])
		}
	}
	t.Logf("%d positions round-tripped", positions)
}

func TestSetResult(t *testing.T) {
	var rec [RecordSize]byte
	if err := Encode(rec[:], board.NewInitialBoard(), Label{Result: Draw}); err != nil {
		t.Fatal(err)
	}
	SetResult(rec[:], WhiteWin)
	if _, l, err := DecodeFEN(rec[:]); err != nil || l.Result != WhiteWin {
		t.Fatalf("result %d err %v, want WhiteWin", l.Result, err)
	}
}

func TestRejectsBadInput(t *testing.T) {
	b, _ := board.FromFEN("rnbqkbnr/ppp1p1pp/8/3pPp2/8/8/PPPP1PPP/RNBQKBNR w KQkq f6 0 3")
	b.EnPassant, _ = board.ParseSquare("f3") // wrong rank for white to move
	var rec [RecordSize]byte
	if err := Encode(rec[:], b, Label{}); err == nil {
		t.Fatal("en passant on the wrong rank must be rejected")
	}
	if err := Encode(rec[:], board.NewInitialBoard(), Label{Result: 3}); err == nil {
		t.Fatal("result 3 must be rejected")
	}
	if err := Encode(rec[:], board.NewInitialBoard(), Label{}); err != nil {
		t.Fatal(err)
	}
	rec[offPieces] = 0x06 // code 6 is unused
	if _, _, err := DecodeFEN(rec[:]); err == nil {
		t.Fatal("piece code 6 must be rejected")
	}
}
