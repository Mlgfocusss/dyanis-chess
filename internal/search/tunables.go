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
}

// Tunables lists the tunable parameters (the live values are in Value).
func Tunables() []Tunable { return tunables }

// SetTunable sets the named parameter, clamped to its range. It reports
// whether the name exists.
func SetTunable(name string, v int) bool {
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
