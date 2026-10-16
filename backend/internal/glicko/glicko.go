// Package glicko is the Glicko-2 rating system (Mark Glickman, 2013), the one chess.com and Lichess
// are built on: a rating is three numbers, the rating itself, its uncertainty (the rating
// deviation, RD) and the volatility, how erratically the person's results swing. A newcomer has a
// large RD, so the first games move the rating a lot; it shrinks as games are played, and grows
// again while the person is away.
package glicko

import "math"

// The constants of the system.
const (
	// Scale turns Glicko-2's internal scale into the familiar one (rating = 1500 + Scale*mu).
	Scale = 173.7178
	// Tau limits how fast the volatility may change; 0.3 to 1.2 is Glickman's advice, chess sites use about 0.5.
	Tau = 0.5

	StartRating     = 1200.0 // a new player; chess.com starts people at 1200 by default
	StartRD         = 350.0
	StartVolatility = 0.06

	MinRD = 40.0  // a rating is never taken as exact
	MaxRD = 350.0 // an unrated player's uncertainty: it does not grow beyond this
	// Provisional is the RD above which a rating is still a rough guess (chess.com and Lichess mark it with "?").
	Provisional = 110.0

	epsilon = 1e-6
)

// Rating is a player's Glicko-2 rating.
type Rating struct {
	R     float64 // the rating
	RD    float64 // rating deviation
	Sigma float64 // volatility
}

// New is the rating of a player with no games.
func New() Rating { return Rating{R: StartRating, RD: StartRD, Sigma: StartVolatility} }

// Result is one game from the point of view of the player being updated.
type Result struct {
	Opp   Rating
	Score float64 // 1 win, 0.5 draw, 0 loss
}

func g(phi float64) float64 { return 1 / math.Sqrt(1+3*phi*phi/(math.Pi*math.Pi)) }

func expected(mu, muJ, phiJ float64) float64 { return 1 / (1 + math.Exp(-g(phiJ)*(mu-muJ))) }

// Inflate raises the uncertainty for periods (days) without a game: a rating that has not been
// tested for a while is less sure.
func Inflate(r Rating, periods float64) Rating {
	if periods <= 0 {
		return r
	}
	phi := r.RD / Scale
	phi = math.Sqrt(phi*phi + r.Sigma*r.Sigma*periods)
	r.RD = math.Min(MaxRD, phi*Scale)
	return r
}

// Update gives the new rating after the results (all counted as one rating period). With no
// results only the uncertainty grows, as in the original paper.
func Update(r Rating, results []Result) Rating {
	mu, phi := (r.R-1500)/Scale, r.RD/Scale
	if len(results) == 0 {
		phi = math.Sqrt(phi*phi + r.Sigma*r.Sigma)
		return clamp(Rating{R: r.R, RD: phi * Scale, Sigma: r.Sigma})
	}
	var vInv, sum float64
	for _, o := range results {
		muJ, phiJ := (o.Opp.R-1500)/Scale, o.Opp.RD/Scale
		gj := g(phiJ)
		e := expected(mu, muJ, phiJ)
		vInv += gj * gj * e * (1 - e)
		sum += gj * (o.Score - e)
	}
	v := 1 / vInv
	delta := v * sum

	// the new volatility, by the Illinois iteration of the paper
	a := math.Log(r.Sigma * r.Sigma)
	f := func(x float64) float64 {
		ex := math.Exp(x)
		d := phi*phi + v + ex
		return ex*(delta*delta-phi*phi-v-ex)/(2*d*d) - (x-a)/(Tau*Tau)
	}
	A := a
	var B float64
	if delta*delta > phi*phi+v {
		B = math.Log(delta*delta - phi*phi - v)
	} else {
		k := 1.0
		for f(a-k*Tau) < 0 {
			k++
		}
		B = a - k*Tau
	}
	fA, fB := f(A), f(B)
	for i := 0; math.Abs(B-A) > epsilon && i < 100; i++ {
		C := A + (A-B)*fA/(fB-fA)
		fC := f(C)
		if fC*fB <= 0 {
			A, fA = B, fB
		} else {
			fA /= 2
		}
		B, fB = C, fC
	}
	sigma := math.Exp(A / 2)

	phiStar := math.Sqrt(phi*phi + sigma*sigma)
	phiNew := 1 / math.Sqrt(1/(phiStar*phiStar)+1/v)
	muNew := mu + phiNew*phiNew*sum
	return clamp(Rating{R: 1500 + Scale*muNew, RD: phiNew * Scale, Sigma: sigma})
}

func clamp(r Rating) Rating {
	r.RD = math.Max(MinRD, math.Min(MaxRD, r.RD))
	return r
}

// IsProvisional says whether the rating is still a rough guess.
func (r Rating) IsProvisional() bool { return r.RD > Provisional }
