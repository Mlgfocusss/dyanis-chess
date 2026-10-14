// Package trainingdata encodes positions for NNUE training as fixed
// 32-byte records (see NNUE.md for the layout): the position itself, not
// network features, so the features can change without regenerating
// data. Shards are plain concatenations of records, readable with
// numpy.memmap; everything about a shard lives in its .json sidecar.
package trainingdata

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"strconv"
	"strings"

	"dyanis-chess/internal/board"
)

// RecordSize is the size of one encoded position in bytes.
const RecordSize = 32

// FormatVersion is written to every shard sidecar; bump it on any layout
// change.
const FormatVersion = 1

// MateScore is the stored score of a position the search scored as a
// forced mate (with FlagMate set); ordinary scores are clamped to
// ±MaxScore so the two never collide.
const (
	MateScore = 32000
	MaxScore  = 31999
)

// Game result from White's point of view, byte 26 of a record.
const (
	BlackWin uint8 = 0
	Draw     uint8 = 1
	WhiteWin uint8 = 2
)

// Bits of the flags byte (27).
const (
	FlagBlackToMove uint8 = 1 << 0
	FlagInCheck     uint8 = 1 << 1
	FlagNoisyBest   uint8 = 1 << 2 // best move is a capture, en passant or promotion
	FlagMate        uint8 = 1 << 3 // score is ±MateScore
)

const (
	offOcc      = 0
	offPieces   = 8
	offScore    = 24
	offResult   = 26
	offFlags    = 27
	offCastleEP = 28
	offHalfmove = 29
	offFullmove = 30
)

// Label is everything in a record that is not the position.
type Label struct {
	Score     int  // White's point of view; ignored in favor of ±MateScore when Mate
	Mate      bool // Score is a forced mate; only its sign is kept
	InCheck   bool
	NoisyBest bool
	Result    uint8 // BlackWin, Draw or WhiteWin
}

// pieceCode is the 4-bit code of a piece: color<<3 | (type-1).
func pieceCode(p board.Piece) byte {
	c := byte(p.Type() - 1)
	if p.Color() == board.Black {
		c |= 8
	}
	return c
}

// Encode writes position b with label l into dst[:RecordSize].
func Encode(dst []byte, b *board.Board, l Label) error {
	if len(dst) < RecordSize {
		return fmt.Errorf("trainingdata: buffer of %d bytes, need %d", len(dst), RecordSize)
	}
	if l.Result > WhiteWin {
		return fmt.Errorf("trainingdata: bad result %d", l.Result)
	}
	rec := dst[:RecordSize]
	clear(rec)

	occ := uint64(b.Occupied())
	if n := bits.OnesCount64(occ); n > 32 {
		return fmt.Errorf("trainingdata: %d pieces, at most 32 fit", n)
	}
	binary.LittleEndian.PutUint64(rec[offOcc:], occ)
	for k, rest := 0, occ; rest != 0; k++ {
		sq := board.Square(bits.TrailingZeros64(rest))
		rest &= rest - 1
		rec[offPieces+k/2] |= pieceCode(b.PieceAt(sq)) << (4 * (k % 2))
	}

	score := l.Score
	if l.Mate {
		score = MateScore
		if l.Score < 0 {
			score = -MateScore
		}
	} else {
		score = min(max(score, -MaxScore), MaxScore)
	}
	binary.LittleEndian.PutUint16(rec[offScore:], uint16(int16(score)))
	rec[offResult] = l.Result

	var flags uint8
	if b.SideToMove == board.Black {
		flags |= FlagBlackToMove
	}
	if l.InCheck {
		flags |= FlagInCheck
	}
	if l.NoisyBest {
		flags |= FlagNoisyBest
	}
	if l.Mate {
		flags |= FlagMate
	}
	rec[offFlags] = flags

	epFile := byte(0)
	if b.EnPassant != board.NoSquare {
		if b.EnPassant.Rank() != epRank(b.SideToMove) {
			return fmt.Errorf("trainingdata: en passant square %v with %v to move", b.EnPassant, b.SideToMove)
		}
		epFile = byte(b.EnPassant.File()) + 1
	}
	rec[offCastleEP] = byte(b.Castling)&0x0f | epFile<<4
	rec[offHalfmove] = byte(min(max(b.HalfmoveClock, 0), 255))
	binary.LittleEndian.PutUint16(rec[offFullmove:], uint16(min(max(b.FullmoveNumber, 0), 65535)))
	return nil
}

// epRank is the rank (0-based) an en passant target square lies on when
// side is to move: behind a black pawn on rank 6, or a white one on rank 3.
func epRank(side board.Color) int {
	if side == board.White {
		return 5
	}
	return 2
}

// SetResult overwrites the result byte of an encoded record — the game's
// outcome is only known after all its positions have been encoded.
func SetResult(rec []byte, result uint8) {
	rec[offResult] = result
}

var codeLetters = [16]byte{'P', 'N', 'B', 'R', 'Q', 'K', 0, 0, 'p', 'n', 'b', 'r', 'q', 'k', 0, 0}

// DecodeFEN returns the position of a record as FEN, and its label.
func DecodeFEN(rec []byte) (string, Label, error) {
	if len(rec) < RecordSize {
		return "", Label{}, fmt.Errorf("trainingdata: record of %d bytes, need %d", len(rec), RecordSize)
	}
	var squares [64]byte
	occ := binary.LittleEndian.Uint64(rec[offOcc:])
	if n := bits.OnesCount64(occ); n > 32 {
		return "", Label{}, fmt.Errorf("trainingdata: %d occupied squares", n)
	}
	for k, rest := 0, occ; rest != 0; k++ {
		sq := bits.TrailingZeros64(rest)
		rest &= rest - 1
		code := rec[offPieces+k/2] >> (4 * (k % 2)) & 0x0f
		if codeLetters[code] == 0 {
			return "", Label{}, fmt.Errorf("trainingdata: bad piece code %d", code)
		}
		squares[sq] = codeLetters[code]
	}

	var sb strings.Builder
	for rank := 7; rank >= 0; rank-- {
		empty := 0
		for file := 0; file < 8; file++ {
			c := squares[rank*8+file]
			if c == 0 {
				empty++
				continue
			}
			if empty > 0 {
				sb.WriteByte(byte('0' + empty))
				empty = 0
			}
			sb.WriteByte(c)
		}
		if empty > 0 {
			sb.WriteByte(byte('0' + empty))
		}
		if rank > 0 {
			sb.WriteByte('/')
		}
	}

	flags := rec[offFlags]
	side := board.White
	if flags&FlagBlackToMove != 0 {
		side = board.Black
		sb.WriteString(" b ")
	} else {
		sb.WriteString(" w ")
	}

	castle := board.CastlingRights(rec[offCastleEP] & 0x0f)
	if castle == 0 {
		sb.WriteByte('-')
	}
	for _, c := range []struct {
		r board.CastlingRights
		l byte
	}{{board.WhiteKingside, 'K'}, {board.WhiteQueenside, 'Q'}, {board.BlackKingside, 'k'}, {board.BlackQueenside, 'q'}} {
		if castle&c.r != 0 {
			sb.WriteByte(c.l)
		}
	}

	sb.WriteByte(' ')
	epFile := int(rec[offCastleEP] >> 4)
	switch {
	case epFile == 0:
		sb.WriteByte('-')
	case epFile <= 8:
		sb.WriteString(board.MakeSquare(epFile-1, epRank(side)).String())
	default:
		return "", Label{}, fmt.Errorf("trainingdata: bad en passant file %d", epFile)
	}

	sb.WriteString(" " + strconv.Itoa(int(rec[offHalfmove])))
	sb.WriteString(" " + strconv.Itoa(int(binary.LittleEndian.Uint16(rec[offFullmove:]))))

	l := Label{
		Score:     int(int16(binary.LittleEndian.Uint16(rec[offScore:]))),
		Mate:      flags&FlagMate != 0,
		InCheck:   flags&FlagInCheck != 0,
		NoisyBest: flags&FlagNoisyBest != 0,
		Result:    rec[offResult],
	}
	if l.Result > WhiteWin {
		return "", Label{}, fmt.Errorf("trainingdata: bad result %d", l.Result)
	}
	return sb.String(), l, nil
}

// Decode is DecodeFEN parsed into a Board.
func Decode(rec []byte) (*board.Board, Label, error) {
	fen, l, err := DecodeFEN(rec)
	if err != nil {
		return nil, Label{}, err
	}
	b, err := board.FromFEN(fen)
	return b, l, err
}
