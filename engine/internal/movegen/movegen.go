// Package movegen generates legal chess moves for a given position.
//
// Strategy: moves are generated directly as legal, using a
// checkers-mask and per-piece pin masks (see GenerateLegalMoves)
// rather than generating pseudo-legal moves and filtering them with
// make/unmake. Correctness is verified by the perft tests against
// known reference counts.
//
// Move generation is bitboard-driven (see internal/board's Bitboard,
// the non-sliding attack tables, and the magic bitboards in
// board/magic.go): for every piece type, start from that type's own
// bitboard (board.Pieces) and pop one square at a time rather than
// scanning all 64 squares checking what's on each one, and get each
// piece's destination set from a table/magic lookup rather than
// walking offsets or rays by hand. This replaced an earlier mailbox-
// scan version once profiling showed GenerateLegalMoves/
// IsSquareAttacked as the dominant cost at real search depths — see
// TECHNICAL.md for the full migration writeup.
package movegen

import "dyanis-chess/engine/internal/board"

func onBoard(file, rank int) bool {
	return file >= 0 && file < 8 && rank >= 0 && rank < 8
}

// GenerateLegalMoves returns every legal move for the side to move,
// generated directly as legal rather than filtered down to legal:
// Checkers()/Pins() (above) are computed once per call and turned
// into bitboard masks — checkMask (capture-the-checker-or-block-the-
// check, or unrestricted if not in check) and a per-square pin mask
// (a pinned piece may only move within the line to its own king, or
// capture the pinner) — which every piece's own attack/move bitboard
// gets ANDed against before any move ever gets built. No MakeMove/
// UnmakeMove anywhere in this function except the one, deliberately
// narrow exception in appendLegalEnPassant — see its own comment for
// why that specific case still needs it.
//
// Double check (Checkers().Count() >= 2) short-circuits immediately
// after king moves: no single capture or block can ever address two
// simultaneous attackers, so nothing else is legal.
func GenerateLegalMoves(b *board.Board) []board.Move {
	return generateLegal(b, make([]board.Move, 0, 40), false)
}

// AppendLegalMoves is GenerateLegalMoves appending onto dst (typically
// buf[:0] of a caller-owned stack array), so a hot caller pays no heap
// allocation per call. Moves come out in exactly the same order.
func AppendLegalMoves(b *board.Board, dst []board.Move) []board.Move {
	return generateLegal(b, dst, false)
}

// HasLegalMove reports whether the side to move has at least one legal
// move. It runs the same generator as GenerateLegalMoves (so legality is
// decided in exactly one place) but returns as soon as one move is found.
func HasLegalMove(b *board.Board) bool {
	var buf [32]board.Move // one piece's worth of moves is the most that can precede the early exit
	return len(generateLegal(b, buf[:0], true)) > 0
}

// generateLegal is GenerateLegalMoves appending onto moves; with stopAtFirst
// it returns early with a longer slice as soon as any legal move exists (only
// emptiness is meaningful).
func generateLegal(b *board.Board, moves []board.Move, stopAtFirst bool) []board.Move {
	us := b.SideToMove
	kingSq := b.KingSquare(us)
	base := len(moves)

	checkMask := ^board.Bitboard(0)
	// pinAllowed[sq] is meaningful only for squares in pinned: where that
	// pinned piece may still go. Unpinned pieces are unrestricted.
	var pinAllowed [64]board.Bitboard
	var pinned board.Bitboard
	var checkers board.Bitboard

	// A side to move with no king at all is never a real game state
	// (MakeMove never creates or removes a king), but this codebase
	// has several test helpers elsewhere that build minimal, partial
	// positions (e.g. eval_test.go's emptyBoard-based ones) which may
	// not always place one. Check/pin logic is meaningless without a
	// king to check or pin relative to, so this branch is skipped
	// entirely rather than calling appendKingMoves/Checkers/Pins with
	// a NoSquare king — every one of those would eventually index
	// board's attack tables with Square(-1) and panic. Every other
	// piece still moves exactly as freely as it does in the normal
	// case below; checkMask/pinMask simply stay fully unrestricted.
	if kingSq != board.NoSquare {
		moves = appendKingMoves(b, kingSq, us, moves)
		if stopAtFirst && len(moves) > base {
			return moves
		}

		checkers = Checkers(b)
		if checkers.Count() >= 2 {
			return moves
		}
		if checkers != 0 {
			checkerSq, _ := checkers.PopLSB()
			var checkerBit board.Bitboard
			checkerBit.Set(checkerSq)
			checkMask = checkerBit | board.SquaresBetween(kingSq, checkerSq)
		}
		pinned = computePins(b, kingSq, &pinAllowed)
	}

	occ := b.Occupied()
	own := b.OccupiedBy(us)

	addNonKing := func(from board.Square, attacks board.Bitboard) {
		targets := attacks &^ own & checkMask
		if pinned.Test(from) {
			targets &= pinAllowed[from]
		}
		moves = appendBitboardMoves(moves, from, targets, b, us)
	}

	knights := b.Pieces(board.Knight, us)
	for knights != 0 {
		var sq board.Square
		sq, knights = knights.PopLSB()
		addNonKing(sq, board.KnightAttacks(sq))
		if stopAtFirst && len(moves) > base {
			return moves
		}
	}
	bishops := b.Pieces(board.Bishop, us)
	for bishops != 0 {
		var sq board.Square
		sq, bishops = bishops.PopLSB()
		addNonKing(sq, board.BishopAttacks(sq, occ))
		if stopAtFirst && len(moves) > base {
			return moves
		}
	}
	rooks := b.Pieces(board.Rook, us)
	for rooks != 0 {
		var sq board.Square
		sq, rooks = rooks.PopLSB()
		addNonKing(sq, board.RookAttacks(sq, occ))
		if stopAtFirst && len(moves) > base {
			return moves
		}
	}
	queens := b.Pieces(board.Queen, us)
	for queens != 0 {
		var sq board.Square
		sq, queens = queens.PopLSB()
		addNonKing(sq, board.QueenAttacks(sq, occ))
		if stopAtFirst && len(moves) > base {
			return moves
		}
	}

	pawns := b.Pieces(board.Pawn, us)
	for pawns != 0 {
		var sq board.Square
		sq, pawns = pawns.PopLSB()
		pinMask := ^board.Bitboard(0)
		if pinned.Test(sq) {
			pinMask = pinAllowed[sq]
		}
		moves = generateLegalPawnMoves(b, sq, us, checkMask, pinMask, moves)
		if stopAtFirst && len(moves) > base {
			return moves
		}
	}

	if checkers == 0 {
		moves = appendCastlingMoves(b, us, moves)
	}

	return moves
}

// appendKingMoves appends the side to move's legal king moves. Unlike
// every other piece, a king's own legality can't be reduced to a
// simple mask ANDed in up front — each candidate destination has to
// be checked individually for safety, AND checked against occupancy
// with the king itself removed: a king stepping straight back along
// the very ray a rook or queen is checking it on must not see its own
// old square as still blocking that ray (the classic "king can't
// retreat along the checking line" bug) — squareAttackedWithOccupancy
// exists specifically to make that occupancy explicit rather than
// always reading b.Occupied(), which would get this wrong.
func appendKingMoves(b *board.Board, kingSq board.Square, us board.Color, moves []board.Move) []board.Move {
	if kingSq == board.NoSquare {
		return moves
	}
	them := us.Opposite()
	var kingBit board.Bitboard
	kingBit.Set(kingSq)
	occWithoutKing := b.Occupied() &^ kingBit
	enemy := b.OccupiedBy(them)

	targets := board.KingAttacks(kingSq) &^ b.OccupiedBy(us)
	for targets != 0 {
		var to board.Square
		to, targets = targets.PopLSB()
		if squareAttackedWithOccupancy(b, to, them, occWithoutKing) {
			continue
		}
		flag := board.Quiet
		if enemy.Test(to) {
			flag = board.Capture
		}
		moves = append(moves, board.Move{From: kingSq, To: to, Flag: flag})
	}
	return moves
}

// generateLegalPawnMoves appends from's legal pawn moves (pushes,
// captures, promotions): every destination gets checkMask&pinMask
// applied before a Move is built, and en passant is routed through
// appendLegalEnPassant (see that function's comment for why it needs
// special handling neither mask alone can give it).
func generateLegalPawnMoves(b *board.Board, from board.Square, us board.Color, checkMask, pinMask board.Bitboard, moves []board.Move) []board.Move {
	f, r := from.File(), from.Rank()
	allowed := checkMask & pinMask

	forward := 1
	startRank := 1
	promoRank := 7
	if us == board.Black {
		forward = -1
		startRank = 6
		promoRank = 0
	}

	addForwardOrPromo := func(to board.Square, flag board.MoveFlag) {
		if to.Rank() == promoRank {
			for _, pt := range promotionTypes {
				pf := board.Promotion
				if flag == board.Capture {
					pf = board.PromotionCapture
				}
				moves = append(moves, board.Move{From: from, To: to, Flag: pf, Promotion: pt})
			}
		} else {
			moves = append(moves, board.Move{From: from, To: to, Flag: flag})
		}
	}

	occ := b.Occupied()

	oneAheadRank := r + forward
	if onBoard(f, oneAheadRank) {
		oneAhead := board.MakeSquare(f, oneAheadRank)
		if !occ.Test(oneAhead) {
			if allowed.Test(oneAhead) {
				addForwardOrPromo(oneAhead, board.Quiet)
			}
			if r == startRank {
				twoAhead := board.MakeSquare(f, r+2*forward)
				if !occ.Test(twoAhead) && allowed.Test(twoAhead) {
					moves = append(moves, board.Move{From: from, To: twoAhead, Flag: board.DoublePawnPush})
				}
			}
		}
	}

	enemy := b.OccupiedBy(us.Opposite())
	captureTargets := board.PawnAttacks(us, from)
	for captureTargets != 0 {
		var to board.Square
		to, captureTargets = captureTargets.PopLSB()

		if enemy.Test(to) {
			if allowed.Test(to) {
				addForwardOrPromo(to, board.Capture)
			}
		} else if to == b.EnPassant {
			moves = appendLegalEnPassant(b, from, to, us, checkMask, pinMask, moves)
		}
	}

	return moves
}

// appendLegalEnPassant handles en passant's two ways of not fitting
// the ordinary checkMask/pinMask story:
//
//  1. checkMask alone would wrongly reject a legal capture of a
//     checking pawn: checkMask covers the CHECKER's square, but en
//     passant's own To square is the empty square behind it, not the
//     checker's square itself — so this checks both To (does it block
//     a slider's check) and the actually-captured square (does it
//     capture the checker) rather than just To.
//  2. Neither checkMask nor a per-piece pin mask can catch the classic
//     en passant discovered check: capturing en passant removes BOTH
//     pawns from the same rank in one move, which can expose the king
//     to a rook/queen along that rank even though neither pawn was
//     individually pinned beforehand (the pin only exists once both
//     are gone together). Rare enough — at most one candidate per
//     position — that falling back to one real MakeMove/
//     IsSquareAttacked/UnmakeMove cycle here, exactly the mechanism
//     this whole rewrite exists to stop paying for on every move,
//     costs nothing measurable. Standard handling for this specific
//     case in essentially every bitboard move generator, not a
//     shortcut unique to this one.
func appendLegalEnPassant(b *board.Board, from, to board.Square, us board.Color, checkMask, pinMask board.Bitboard, moves []board.Move) []board.Move {
	capSq := board.MakeSquare(to.File(), from.Rank())

	if !pinMask.Test(to) {
		return moves
	}
	if !checkMask.Test(to) && !checkMask.Test(capSq) {
		return moves
	}

	m := board.Move{From: from, To: to, Flag: board.EnPassantCapture}

	undo := b.MakeMove(m)
	stillLegal := !IsSquareAttacked(b, b.KingSquare(us), b.SideToMove)
	b.UnmakeMove(m, undo)

	if stillLegal {
		moves = append(moves, m)
	}
	return moves
}

// GenerateLegalCaptures returns every legal CAPTURE for the side to
// move (regular captures, en passant, and promotion-captures) — NOT
// "every legal move filtered down to captures": the pseudo-legal step
// below generates straight from each piece type's attack bitboard
// ANDed with enemy occupancy, so quiet moves (pawn pushes, non-
// capturing knight/bishop/rook/queen/king destinations, castling)
// never get generated in the first place, rather than being generated
// and then discarded. Existed for quiescenceSearch (see search.go),
// which only ever wants captures when the side to move isn't in check
// — quiescence nodes vastly outnumber ordinary search nodes, and the
// large majority of moves in most positions are quiet, so skipping
// quiet-move generation entirely there was a real, measurable chunk
// of total search time (see TECHNICAL.md's migration notes).
//
// Deliberately NOT a substitute for GenerateLegalMoves when checking
// whether the side to move has ANY legal move at all (checkmate/
// stalemate detection): a position can easily have zero legal
// CAPTURES while still having plenty of legal quiet moves, so an
// empty result here does not mean "no legal moves", the way an empty
// GenerateLegalMoves result does.
func GenerateLegalCaptures(b *board.Board) []board.Move {
	pseudo := generatePseudoLegalCaptures(b, make([]board.Move, 0, 8))
	legal := make([]board.Move, 0, len(pseudo))
	us := b.SideToMove

	for _, m := range pseudo {
		undo := b.MakeMove(m)
		if !IsSquareAttacked(b, b.KingSquare(us), b.SideToMove) {
			legal = append(legal, m)
		}
		b.UnmakeMove(m, undo)
	}
	return legal
}

// generatePseudoLegalCaptures generates pseudo-legal captures only:
// one pass per piece type over its bitboard, with every attack
// bitboard ANDed with enemy occupancy, so quiet destinations are never
// even considered. No castling (never a capture) and no quiet pawn
// pushes — pawns get their own helper, appendPawnCaptures, since a
// pawn capture can promote and can be en passant, neither of which the
// generic "one Move per set bit" shape below handles.
func generatePseudoLegalCaptures(b *board.Board, moves []board.Move) []board.Move {
	us := b.SideToMove
	enemy := b.OccupiedBy(us.Opposite())
	occ := b.Occupied()

	pawns := b.Pieces(board.Pawn, us)
	for pawns != 0 {
		var from board.Square
		from, pawns = pawns.PopLSB()
		moves = appendPawnCaptures(b, from, us, enemy, moves)
	}

	knights := b.Pieces(board.Knight, us)
	for knights != 0 {
		var sq board.Square
		sq, knights = knights.PopLSB()
		moves = appendCaptureBits(moves, sq, board.KnightAttacks(sq)&enemy)
	}

	bishops := b.Pieces(board.Bishop, us)
	for bishops != 0 {
		var sq board.Square
		sq, bishops = bishops.PopLSB()
		moves = appendCaptureBits(moves, sq, board.BishopAttacks(sq, occ)&enemy)
	}

	rooks := b.Pieces(board.Rook, us)
	for rooks != 0 {
		var sq board.Square
		sq, rooks = rooks.PopLSB()
		moves = appendCaptureBits(moves, sq, board.RookAttacks(sq, occ)&enemy)
	}

	queens := b.Pieces(board.Queen, us)
	for queens != 0 {
		var sq board.Square
		sq, queens = queens.PopLSB()
		moves = appendCaptureBits(moves, sq, board.QueenAttacks(sq, occ)&enemy)
	}

	kings := b.Pieces(board.King, us)
	for kings != 0 {
		var sq board.Square
		sq, kings = kings.PopLSB()
		moves = appendCaptureBits(moves, sq, board.KingAttacks(sq)&enemy)
	}

	return moves
}

// appendCaptureBits appends one Capture-flagged move per set bit in
// targets — targets is always already ANDed with enemy occupancy by
// every caller above, so every bit here is a genuine capture, no
// Quiet/Capture branching needed the way appendBitboardMoves has.
func appendCaptureBits(moves []board.Move, from board.Square, targets board.Bitboard) []board.Move {
	for targets != 0 {
		var to board.Square
		to, targets = targets.PopLSB()
		moves = append(moves, board.Move{From: from, To: to, Flag: board.Capture})
	}
	return moves
}

// appendPawnCaptures appends from's pawn captures (diagonal captures,
// promotion-captures, and en passant) onto moves. board.PawnAttacks
// gives both diagonal targets in one lookup; each is then either a
// genuine capture (enemy-occupied — possibly with promotion, if it
// lands on the back rank), the en passant square specifically (the
// one legal pawn "capture" whose target square ISN'T enemy-occupied),
// or neither, in which case it's simply not a legal pawn capture from
// here and gets skipped.
func appendPawnCaptures(b *board.Board, from board.Square, us board.Color, enemy board.Bitboard, moves []board.Move) []board.Move {
	promoRank := 7
	if us == board.Black {
		promoRank = 0
	}

	targets := board.PawnAttacks(us, from)
	for targets != 0 {
		var to board.Square
		to, targets = targets.PopLSB()

		switch {
		case enemy.Test(to) && to.Rank() == promoRank:
			for _, pt := range promotionTypes {
				moves = append(moves, board.Move{From: from, To: to, Flag: board.PromotionCapture, Promotion: pt})
			}
		case enemy.Test(to):
			moves = append(moves, board.Move{From: from, To: to, Flag: board.Capture})
		case to == b.EnPassant:
			moves = append(moves, board.Move{From: from, To: to, Flag: board.EnPassantCapture})
		}
	}
	return moves
}

// appendBitboardMoves appends one Quiet or Capture move per set bit in
// targets — the common shape shared by knight/king/bishop/rook/queen
// move generation, now that each of those is just "start from a
// table/magic attack bitboard, mask off squares the mover's own side
// already occupies" (every caller arrives at targets that way, via
// `attacks &^ b.OccupiedBy(us)`, so a set bit here always means either
// empty or an enemy piece — never a friendly one). Pawns don't go
// through this: pushes aren't attacks (no table for them) and captures
// need extra promotion/en-passant branching, so pawns have their own
// functions (generateLegalPawnMoves, appendPawnCaptures).
func appendBitboardMoves(moves []board.Move, from board.Square, targets board.Bitboard, b *board.Board, us board.Color) []board.Move {
	enemy := b.OccupiedBy(us.Opposite())
	for targets != 0 {
		var to board.Square
		to, targets = targets.PopLSB()
		flag := board.Quiet
		if enemy.Test(to) {
			flag = board.Capture
		}
		moves = append(moves, board.Move{From: from, To: to, Flag: flag})
	}
	return moves
}

var promotionTypes = []board.PieceType{board.Queen, board.Rook, board.Bishop, board.Knight}

// appendCastlingMoves appends the legal castling moves onto moves: it checks
// the standard castling preconditions — rights still held, squares between
// king and rook empty, and the king does not start, pass through, or land on
// an attacked square. (Rights are already revoked elsewhere once the king or
// rook has moved or been captured — see board.MakeMove.)
func appendCastlingMoves(b *board.Board, us board.Color, moves []board.Move) []board.Move {
	rank := 0
	if us == board.Black {
		rank = 7
	}
	kingSq := board.MakeSquare(4, rank)

	if b.PieceAt(kingSq) != board.MakePiece(board.King, us) {
		return moves // king not on its home square, can't castle
	}
	if IsSquareAttacked(b, kingSq, us.Opposite()) {
		return moves // can't castle out of check
	}

	occ := b.Occupied()

	kingsideRight, queensideRight := board.WhiteKingside, board.WhiteQueenside
	if us == board.Black {
		kingsideRight, queensideRight = board.BlackKingside, board.BlackQueenside
	}

	if b.Castling&kingsideRight != 0 {
		fSq := board.MakeSquare(5, rank)
		gSq := board.MakeSquare(6, rank)
		if !occ.Test(fSq) && !occ.Test(gSq) &&
			!IsSquareAttacked(b, fSq, us.Opposite()) && !IsSquareAttacked(b, gSq, us.Opposite()) {
			moves = append(moves, board.Move{From: kingSq, To: gSq, Flag: board.CastleKingside})
		}
	}
	if b.Castling&queensideRight != 0 {
		dSq := board.MakeSquare(3, rank)
		cSq := board.MakeSquare(2, rank)
		bSq := board.MakeSquare(1, rank)
		if !occ.Test(dSq) && !occ.Test(cSq) && !occ.Test(bSq) &&
			!IsSquareAttacked(b, dSq, us.Opposite()) && !IsSquareAttacked(b, cSq, us.Opposite()) {
			moves = append(moves, board.Move{From: kingSq, To: cSq, Flag: board.CastleQueenside})
		}
	}
	return moves
}

// AttackersTo returns the bitboard of every by-colored piece
// currently attacking sq — the same five piece-type checks
// IsSquareAttacked makes, just accumulated into one Bitboard instead
// of short-circuiting on the first hit found. IsSquareAttacked(b, sq,
// by) is exactly AttackersTo(b, sq, by) != 0; this is for callers that
// also need to know WHICH square(s) are attacking, not just whether
// any is — check detection (Checkers, below) and, planned next,
// pin detection for legal-move generation (see TECHNICAL.md).
//
// Deliberately not IsSquareAttacked's own implementation in terms of
// this — IsSquareAttacked stays its separate, early-exiting self
// (cheap pawn/knight/king checks first, magic sliding lookups only if
// nothing cheaper already hit) since it's still called on real hot
// paths (GenerateLegalMoves' per-candidate legality filter, castling
// safety) where computing all five kinds of attacker unconditionally,
// every time, would be strictly more work for no benefit.
func AttackersTo(b *board.Board, sq board.Square, by board.Color) board.Bitboard {
	if sq == board.NoSquare {
		return 0
	}

	attackers := board.PawnAttacks(by.Opposite(), sq) & b.Pieces(board.Pawn, by)
	attackers |= board.KnightAttacks(sq) & b.Pieces(board.Knight, by)
	attackers |= board.KingAttacks(sq) & b.Pieces(board.King, by)

	occ := b.Occupied()
	attackers |= board.RookAttacks(sq, occ) & (b.Pieces(board.Rook, by) | b.Pieces(board.Queen, by))
	attackers |= board.BishopAttacks(sq, occ) & (b.Pieces(board.Bishop, by) | b.Pieces(board.Queen, by))

	return attackers
}

// PinInfo describes one pinned piece belonging to the side to move:
// which square it's on, and the full bitboard of squares it's still
// allowed to move to while pinned — the line between it and its own
// king, plus the pinning piece's own square (capturing it). Any
// square outside AllowedTo is illegal for this piece regardless of
// what its normal movement pattern would otherwise permit, since
// going there would expose the king to the pinner.
type PinInfo struct {
	Square    board.Square
	AllowedTo board.Bitboard
}

// Pins returns every pinned piece belonging to the side to move.
//
// Approach: first find every enemy slider that shares a rook-ray or
// bishop-ray with our king on a completely EMPTY board (occupancy 0)
// — board.RookAttacks/BishopAttacks with occ=0 give the full
// unobstructed line in every direction, so ANDing that against enemy
// rooks/queens/bishops/queens finds every slider anywhere on one of
// the king's 8 lines, however far away and regardless of what's
// actually in between. Call each one a "sniper" — a candidate pinner,
// not yet a confirmed one.
//
// Then, for each sniper, board.SquaresBetween(kingSq, sniperSq) gives
// exactly the squares a blocker could occupy on that specific line,
// and ANDing that against the real board occupancy counts how many
// pieces actually sit there:
//   - zero: nothing blocks that line at all — the king is in check
//     from this slider (Checkers already covers that; not a pin)
//   - two or more: the line is blocked by more than one piece, so
//     nothing on it is pinned — removing any one of them still leaves
//     another piece in the way
//   - exactly one, and it's an ENEMY piece: that piece is simply in
//     its own slider's way, not something of ours being pinned
//   - exactly one, and it's OURS: that's a pin — this piece may only
//     ever move within the line between it and the king, or capture
//     the sniper itself
func Pins(b *board.Board) []PinInfo {
	kingSq := b.KingSquare(b.SideToMove)
	if kingSq == board.NoSquare {
		return nil
	}
	var allowed [64]board.Bitboard
	pinned := computePins(b, kingSq, &allowed)

	var pins []PinInfo
	for pinned != 0 {
		var sq board.Square
		sq, pinned = pinned.PopLSB()
		pins = append(pins, PinInfo{Square: sq, AllowedTo: allowed[sq]})
	}
	return pins
}

// computePins is Pins without the allocation: it returns the set of pinned
// squares and writes each one's allowed-destination mask into allowed[sq]
// (entries for unpinned squares are left untouched).
func computePins(b *board.Board, kingSq board.Square, allowed *[64]board.Bitboard) (pinned board.Bitboard) {
	us := b.SideToMove
	them := us.Opposite()
	occ := b.Occupied()
	own := b.OccupiedBy(us)

	rookSnipers := board.RookAttacks(kingSq, 0) & (b.Pieces(board.Rook, them) | b.Pieces(board.Queen, them))
	bishopSnipers := board.BishopAttacks(kingSq, 0) & (b.Pieces(board.Bishop, them) | b.Pieces(board.Queen, them))
	snipers := rookSnipers | bishopSnipers

	for snipers != 0 {
		var sniperSq board.Square
		sniperSq, snipers = snipers.PopLSB()

		line := board.SquaresBetween(kingSq, sniperSq)
		blockers := line & occ
		if blockers.Count() != 1 || blockers&own == 0 {
			continue
		}
		// A king can never legitimately be "the" pinned piece here —
		// the only way one shows up as the sole blocker on our own
		// king's outgoing ray is a second king of the same color
		// somewhere else on the board, which real play can never
		// produce (MakeMove never creates or removes a king). Skip
		// explicitly rather than leaning on that invariant silently
		// holding, in case this ever runs against a malformed board.
		if blockers&b.Pieces(board.King, us) != 0 {
			continue
		}

		pinnedSq, _ := blockers.PopLSB()
		var sniperBit board.Bitboard
		sniperBit.Set(sniperSq)
		allowed[pinnedSq] = line | sniperBit
		pinned.Set(pinnedSq)
	}
	return pinned
}

// Checkers returns the bitboard of every enemy piece currently
// attacking the side-to-move's king: empty if not in check, one bit
// for an ordinary check, two bits for a double check. Double check is
// the case a future pin-aware legal-move generator has to special-
// case hardest — with two attackers, no single capture or block can
// ever address both, so a king move is the only legal response — which
// is exactly why this returns the full bitboard rather than just a
// count: Count() tells you 0/1/2, and for the single-check case the
// one remaining bit IS the checking piece's square, needed to build
// the capture-or-block mask the rest of that move generation will
// want.
func Checkers(b *board.Board) board.Bitboard {
	us := b.SideToMove
	return AttackersTo(b, b.KingSquare(us), us.Opposite())
}

// IsSquareAttacked reports whether `sq` is attacked by any piece of
// color `by`. Used both for check detection (is my king attacked?)
// and for castling legality (are the king's transit squares safe?).
//
// Every piece type goes through a bitboard lookup: pawn/knight/king
// via the precomputed tables (board.PawnAttacks/KnightAttacks/
// KingAttacks), bishop/rook/queen via magic bitboards
// (board.RookAttacks/BishopAttacks) — an O(1) multiply+shift+array-
// read for every piece type, no ray walk left anywhere in this
// function. This used to be the single biggest chunk of the profile
// that motivated the whole migration (see TECHNICAL.md).
func IsSquareAttacked(b *board.Board, sq board.Square, by board.Color) bool {
	return squareAttackedWithOccupancy(b, sq, by, b.Occupied())
}

// squareAttackedWithOccupancy is IsSquareAttacked's actual logic,
// parameterized on occupancy explicitly instead of always reading
// b.Occupied() — every existing caller through IsSquareAttacked above
// gets identical behavior (same early-exit order, same cost) since it
// just passes b.Occupied() straight through. The one caller that
// needs something else is appendKingMoves: a king's own destination
// safety has to be checked with the king itself removed from
// occupancy, or a slider giving check along the same line the king is
// retreating on would be missed (the king's old square would still
// look like it blocks that ray).
func squareAttackedWithOccupancy(b *board.Board, sq board.Square, by board.Color, occ board.Bitboard) bool {
	if sq == board.NoSquare {
		return false
	}

	// Pawn attacks: see board.PawnAttacks' doc comment for why this
	// is indexed by the OPPOSITE of `by` — it asks "which squares
	// would a by-colored pawn have to stand on to hit sq", then
	// intersects that with where by's pawns actually are.
	if board.PawnAttacks(by.Opposite(), sq)&b.Pieces(board.Pawn, by) != 0 {
		return true
	}

	if board.KnightAttacks(sq)&b.Pieces(board.Knight, by) != 0 {
		return true
	}

	if board.KingAttacks(sq)&b.Pieces(board.King, by) != 0 {
		return true
	}

	if board.RookAttacks(sq, occ)&(b.Pieces(board.Rook, by)|b.Pieces(board.Queen, by)) != 0 {
		return true
	}
	if board.BishopAttacks(sq, occ)&(b.Pieces(board.Bishop, by)|b.Pieces(board.Queen, by)) != 0 {
		return true
	}

	return false
}

// GeneratePseudoLegalCaptures is GenerateLegalCaptures without the
// legality filter: it may include captures that leave the mover's own
// king attacked (pinned pieces, king capturing a defended piece). The
// caller makes each move anyway and rejects it afterward with
// IsSquareAttacked, which saves a second make/unmake per capture.
func GeneratePseudoLegalCaptures(b *board.Board) []board.Move {
	return generatePseudoLegalCaptures(b, make([]board.Move, 0, 8))
}

// AppendPseudoLegalCaptures is GeneratePseudoLegalCaptures appending onto
// dst (typically buf[:0] of a caller-owned stack array): no heap allocation
// per call, same moves in the same order.
func AppendPseudoLegalCaptures(b *board.Board, dst []board.Move) []board.Move {
	return generatePseudoLegalCaptures(b, dst)
}
