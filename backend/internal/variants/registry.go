package variants

import (
	"fmt"
	"sort"
	"sync"
)

// Info describes a variant for the catalogue.
type Info struct {
	ID    string `json:"id"`    // "atomic": letters, digits and "-"; also the folder web/src/variants/<id>
	Name  string `json:"name"`  // "Атомные шахматы"
	Short string `json:"short"` // one sentence of rules for the card
	// Hidden keeps the variant out of the menu and the catalogue (classic is a
	// fixture for tests and for checking the base).
	Hidden bool `json:"hidden,omitempty"`
}

// Variant makes games of one variant.
type Variant interface {
	Info() Info
	// New starts a game. It must be deterministic in its options.
	New(o Options) (State, error)
}

// OptionsChecker is implemented by a Variant that takes parameters from the
// lobby (a Chess960 position number): CheckOptions validates o.Params, fills
// in defaults and may rewrite them. The seed is already set when it runs.
type OptionsChecker interface {
	CheckOptions(o *Options) error
}

var (
	regMu    sync.RWMutex
	registry = map[string]Variant{}
)

// Register adds a variant; call it from init() of the file that defines the
// variant. A second variant with the same id is a programming error.
func Register(v Variant) {
	id := v.Info().ID
	if id == "" {
		panic("variants: a variant without an id")
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := registry[id]; dup {
		panic("variants: " + id + " registered twice")
	}
	registry[id] = v
}

// Get finds a variant by id.
func Get(id string) (Variant, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	v, ok := registry[id]
	return v, ok
}

// All lists the registered variants, hidden ones included, sorted by id.
func All() []Variant {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Variant, 0, len(registry))
	for _, v := range registry {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Info().ID < out[j].Info().ID })
	return out
}

// Prepare checks the options a player asked for and returns the options a
// game is made with (their seed is the one given).
func Prepare(id string, params map[string]any, seed int64) (Variant, Options, error) {
	v, ok := Get(id)
	if !ok {
		return nil, Options{}, fmt.Errorf("такого вида шахмат нет: %q", id)
	}
	o := Options{Seed: seed}
	if len(params) > 0 {
		o.Params = params
	}
	if c, ok := v.(OptionsChecker); ok {
		if err := c.CheckOptions(&o); err != nil {
			return nil, Options{}, err
		}
	} else if len(o.Params) > 0 {
		return nil, Options{}, fmt.Errorf("у вида «%s» нет настроек", v.Info().Name)
	}
	return v, o, nil
}

// Replay rebuilds a game from its options and the actions played, the only
// way a game is restored (see State).
func Replay(id string, o Options, actions []string) (State, error) {
	v, ok := Get(id)
	if !ok {
		return nil, fmt.Errorf("variants: unknown variant %q", id)
	}
	st, err := v.New(o)
	if err != nil {
		return nil, err
	}
	for i, a := range actions {
		if _, err := st.Apply(a); err != nil {
			return nil, fmt.Errorf("variants: %s: action %d (%s): %w", id, i+1, a, err)
		}
	}
	return st, nil
}
