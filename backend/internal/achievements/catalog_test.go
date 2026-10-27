package achievements

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"dyanis-chess/backend/internal/bots"
	"dyanis-chess/backend/internal/store"
)

func TestCatalogIDsAreUniqueAndSane(t *testing.T) {
	idRe := regexp.MustCompile(`^[a-z][a-z0-9_]{1,38}$`)
	seen := map[string]bool{}
	cats := map[Category]bool{}
	for _, c := range Categories {
		cats[c] = true
	}
	perCat := map[Category]int{}
	for _, d := range Catalog {
		if !idRe.MatchString(d.ID) {
			t.Errorf("id %q is not a plain lower-case word", d.ID)
		}
		if seen[d.ID] {
			t.Errorf("id %q repeats", d.ID)
		}
		seen[d.ID] = true
		if !cats[d.Category] {
			t.Errorf("%s: unknown category %q", d.ID, d.Category)
		}
		perCat[d.Category]++
		if d.Tier != Bronze && d.Tier != Silver && d.Tier != Gold {
			t.Errorf("%s: unknown tier %q", d.ID, d.Tier)
		}
		if d.Metric == MAllLessons {
			if d.Target != 0 {
				t.Errorf("%s: the target of the whole course is its number of lessons, not a constant", d.ID)
			}
		} else if d.Target < 1 {
			t.Errorf("%s: the target must be positive", d.ID)
		}
		if got := (Facts{}).Value(d.Metric); got != 0 {
			t.Errorf("%s: a person with nothing has %d of %s", d.ID, got, d.Metric)
		}
	}
	for _, c := range Categories {
		if perCat[c] == 0 {
			t.Errorf("category %s has no badge", c)
		}
	}
	if len(Catalog) < 30 || len(Catalog) > 60 {
		t.Errorf("%d badges: the plan is about 30 to 40", len(Catalog))
	}
}

// Within a metric the badges must get harder with the tier: a higher target never has a lower tier.
func TestTiersFollowTargets(t *testing.T) {
	rank := map[Tier]int{Bronze: 0, Silver: 1, Gold: 2}
	for _, a := range Catalog {
		for _, b := range Catalog {
			if a.Metric == b.Metric && a.Target > 0 && b.Target > a.Target && rank[b.Tier] < rank[a.Tier] {
				t.Errorf("%s (target %d, %s) is harder than %s (%d, %s) but has a lower tier", b.ID, b.Target, b.Tier, a.ID, a.Target, a.Tier)
			}
		}
	}
}

func TestBotTargetsFitTheBots(t *testing.T) {
	n := len(bots.All())
	for _, d := range Catalog {
		if d.Metric == MBotsBeaten && d.Target > n {
			t.Errorf("%s asks for %d bots, there are %d", d.ID, d.Target, n)
		}
	}
	for _, id := range []string{"max", "dionis"} {
		if _, ok := bots.Get(id); !ok {
			t.Errorf("the store counts the bot %q, which does not exist", id)
		}
	}
}

func TestLongGameIsFortyMoves(t *testing.T) {
	// White's 40th move is the 79th half-move: a game that reached it has 40 moves of White
	if LongGamePlies != 79 {
		t.Errorf("LongGamePlies = %d", LongGamePlies)
	}
}

// The words live on the site: every id has a name and a description in Russian and in English.
func TestTextsExistOnTheSite(t *testing.T) {
	for _, lang := range []string{"ru", "en"} {
		path := filepath.Join("..", "..", "..", "web", "src", "i18n", lang, "achievements.js")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("no site dictionary (%v)", err)
		}
		text := string(raw)
		for _, d := range Catalog {
			for _, part := range []string{"name", "desc"} {
				key := `"achievements.` + d.ID + `.` + part + `"`
				if !regexp.MustCompile(regexp.QuoteMeta(key) + `\s*:\s*"[^"]{3,}`).MatchString(text) {
					t.Errorf("%s: no %s for %s", lang, key, d.ID)
				}
			}
		}
		for _, c := range Categories {
			key := `"achievements.cat.` + string(c) + `"`
			if !regexp.MustCompile(regexp.QuoteMeta(key) + `\s*:`).MatchString(text) {
				t.Errorf("%s: no %s", lang, key)
			}
		}
	}
}

func TestStoreFactsAreAllUsed(t *testing.T) {
	// a badge asks for a counter the store really fills in: a spot check of the mapping
	f := Facts{AchievementFacts: store.AchievementFacts{Games: 7, RatingMax: 1650, MaxBeaten: true}}
	if f.Value(MGames) != 7 || f.Value(MRating) != 1650 || f.Value(MBeatMax) != 1 || f.Value(MBotsBeaten) != 0 {
		t.Errorf("values: %d %d %d %d", f.Value(MGames), f.Value(MRating), f.Value(MBeatMax), f.Value(MBotsBeaten))
	}
}
