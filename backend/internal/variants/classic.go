package variants

// classic is ordinary chess on the variants' own board. It is hidden from the
// menu: it exists so that the base can be tested against the engine and played
// end to end through the server and the browser, and as the smallest example of
// a variant. A real variant is a file like this one: Rules, a Variant, init().
type classic struct{}

func (classic) Info() Info {
	return Info{
		ID:     "classic",
		Name:   "Классические шахматы",
		Short:  "Обычные шахматы на доске видов: образец и проверка основы.",
		Hidden: true,
	}
}

func (classic) New(o Options) (State, error) { return NewGame(ClassicRules{}, o) }

func init() { Register(classic{}) }
