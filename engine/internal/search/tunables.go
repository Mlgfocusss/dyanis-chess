package search

// Tunable search parameters, exposed as UCI spin options (see
// internal/uci) so cmd/spsa can tune them by playing matches. The values
// below are the engine's defaults; nothing here changes play until an
// option is set. Options are read by the search without locking, so set
// them only while no search is running (UCI does so before "go").

// lmrBaseX100/lmrDivX100 are the LMR table's constants times 100:
// reduction = int(base + ln(depth)*ln(move)/div).
var (
	lmrBaseX100 = 75
	lmrDivX100  = 223
)

// historyBonusMul/historyBonusCap shape historyBonus: depth²*mul, capped.
var (
	historyBonusMul = 7
	historyBonusCap = 1654
)

// Experiment switches (UCI spin 0/1, deliberately NOT in tunables so
// cmd/spsa and the option list leave them alone): each one turns a
// search-core fix on, so it can be ablated with setoption. All on by default.
var (
	swTTPV      = true // no TT cutoff / window narrowing in PV nodes
	swTTKeep    = true // TT store keeps deeper same-key entries and old moves
	swPartial   = true // use the best root move of an interrupted iteration
	swAspWiden  = true // widen the aspiration window gradually
	swNmp       = true // null move: R grows with depth and eval margin, fail-soft
	swIIR       = true // internal iterative reduction instead of IID
	swDeepPrune = true // futility and LMP reach depth 6
	swCorr      = true // correction history on the static eval
	swSing      = true // singular extensions and multi-cut
	swSingNeg   = true // negative extension of a non-singular TT move that fails high
	swLmrCut    = true // LMR: one ply more in cut nodes
	swLmrCapt   = true // LMR: one ply more when the TT move is a capture
	swLmrFull   = true // separate full-depth move counts for PV and non-PV nodes
	swFutLmr    = true // futility margin by the move's reduced depth
	swSeeQuiet  = true // SEE pruning of quiet moves
	swEvalScale = true // net score scaled by material and the fifty-move counter
	swQFailSoft = true // quiescence returns the best score found, not the window bound
	swQEvasion  = true // check evasions in quiescence sorted, captures first
	swQPromo    = true // quiet queen promotions searched in quiescence
	swHistKeep  = true // history tables live through the whole game
	swHistCapt  = true // capture history in capture ordering
	swHistLmr   = true // history adjusts LMR
	swHistPrune = true // history pruning of quiets at shallow depth
	swLmrPV     = true // one ply less LMR in PV nodes
)

var (
	histLmrDiv      = 4000
	histPruneMargin = 2500
	captHistDiv     = 4
	corrPawnW       = 7000
	corrNonPawnW    = 3000
	nmpBase         = 3
	nmpDivDepth     = 3
	nmpEvalDiv      = 200
	futExtStep      = 150
	lmpExtStep      = 8
	lmrFullPV       = 5
	lmrFullNonPV    = 3
	evalMatBase     = 48  // static eval *= (evalMatBase+material)/(evalMatBase+24)
	evalR50Base     = 200 // static eval *= (evalR50Base-halfmoveClock)/evalR50Base
	seeQuietMul     = 30 // quiet SEE prune: SEE < -seeQuietMul*lmrDepth^2
	singMargin      = 20 // singular beta = ttScore - depth*singMargin/16
)

// singMinDepth is the shallowest remaining depth a singular check runs at.
const singMinDepth = 8

var switches = map[string]*bool{
	"SwTtPv":      &swTTPV,
	"SwTtKeep":    &swTTKeep,
	"SwPartial":   &swPartial,
	"SwAspWiden":  &swAspWiden,
	"SwNmp":       &swNmp,
	"SwIIR":       &swIIR,
	"SwDeepPrune": &swDeepPrune,
	"SwCorr":      &swCorr,
	"SwSing":      &swSing,
	"SwSingNeg":   &swSingNeg,
	"SwLmrCut":    &swLmrCut,
	"SwLmrCapt":   &swLmrCapt,
	"SwLmrFull":   &swLmrFull,
	"SwFutLmr":    &swFutLmr,
	"SwSeeQuiet":  &swSeeQuiet,
	"SwEvalScale": &swEvalScale,
	"SwQFailSoft": &swQFailSoft,
	"SwQEvasion":  &swQEvasion,
	"SwQPromo":    &swQPromo,
	"SwHistKeep":  &swHistKeep,
	"SwHistCapt":  &swHistCapt,
	"SwHistLmr":   &swHistLmr,
	"SwHistPrune": &swHistPrune,
	"SwLmrPV":     &swLmrPV,
}

// Tunable is one named integer parameter with its default and allowed range.
type Tunable struct {
	Name     string
	Value    *int
	Default  int
	Min, Max int
}

var tunables = []Tunable{
	{"RfpMargin", &rfpMargin, 95, 30, 250},
	{"SeePruneMargin", &seePruneMargin, 94, 30, 250},
	{"Futility1", &futilityMargin[1], 135, 50, 400},
	{"Futility2", &futilityMargin[2], 245, 100, 700},
	{"Futility3", &futilityMargin[3], 571, 200, 1000},
	{"Razor1", &razorMargin[1], 235, 100, 600},
	{"Razor2", &razorMargin[2], 602, 200, 900},
	{"Razor3", &razorMargin[3], 943, 400, 1500},
	{"Lmp1", &lmpMoveCountThreshold[1], 5, 3, 16},
	{"Lmp2", &lmpMoveCountThreshold[2], 11, 4, 24},
	{"Lmp3", &lmpMoveCountThreshold[3], 16, 6, 32},
	{"LmrBase", &lmrBaseX100, 75, 20, 150},
	{"LmrDiv", &lmrDivX100, 223, 120, 400},
	{"HistBonusMul", &historyBonusMul, 7, 2, 24},
	{"HistBonusCap", &historyBonusCap, 1654, 400, 3200},
	{"AspirationDelta", &aspirationDelta, 41, 15, 150},
	{"HistLmrDiv", &histLmrDiv, 4000, 1000, 12000},
	{"HistPruneMargin", &histPruneMargin, 2500, 500, 8000},
	{"CaptHistDiv", &captHistDiv, 4, 1, 32},
	{"CorrPawnW", &corrPawnW, 7000, 0, 16000},
	{"CorrNonPawnW", &corrNonPawnW, 3000, 0, 8000},
	{"NmpBase", &nmpBase, 3, 1, 6},
	{"NmpDivDepth", &nmpDivDepth, 3, 2, 8},
	{"NmpEvalDiv", &nmpEvalDiv, 200, 80, 500},
	{"FutExtStep", &futExtStep, 150, 60, 400},
	{"LmpExtStep", &lmpExtStep, 8, 2, 20},
	{"SingMargin", &singMargin, 20, 6, 64},
	{"SeeQuietMul", &seeQuietMul, 30, 8, 100},
	{"EvalMatBase", &evalMatBase, 48, 8, 400},
	{"EvalR50Base", &evalR50Base, 200, 100, 1000},
	{"LmrFullPV", &lmrFullPV, 5, 2, 10},
	{"LmrFullNonPV", &lmrFullNonPV, 3, 2, 10},
}

// Tunables lists the tunable parameters (the live values are in Value).
func Tunables() []Tunable { return tunables }

// SetTunable sets the named parameter, clamped to its range. It reports
// whether the name exists.
func SetTunable(name string, v int) bool {
	if sw, ok := switches[name]; ok {
		*sw = v != 0
		return true
	}
	for _, t := range tunables {
		if t.Name != name {
			continue
		}
		if v < t.Min {
			v = t.Min
		}
		if v > t.Max {
			v = t.Max
		}
		*t.Value = v
		if name == "LmrBase" || name == "LmrDiv" {
			initLMR()
		}
		return true
	}
	return false
}
