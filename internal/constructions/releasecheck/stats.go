package releasecheck

import "math"

// z95 is the normal quantile of a two-sided 95% interval.
const z95 = 1.96

// Wilson returns the proportion k/n and its Wilson 95% score interval, the
// formula of the research analyze.py. It returns NaNs for n = 0.
func Wilson(k, n int) (p, lo, hi float64) {
	if n == 0 {
		return math.NaN(), math.NaN(), math.NaN()
	}
	p = float64(k) / float64(n)
	nf := float64(n)
	d := 1 + z95*z95/nf
	c := (p + z95*z95/(2*nf)) / d
	w := z95 * math.Sqrt(p*(1-p)/nf+z95*z95/(4*nf*nf)) / d
	return p, math.Max(0, c-w), math.Min(1, c+w)
}

// Stratum is one stratum's judged tally and its weight in the corpus.
type Stratum struct {
	True, Judged int
	// Weight is the stratum's research corpus count; Stratified normalises it.
	Weight int
}

// Stratified returns the stratified estimate Σ w_s·p_s over the strata, with a
// normal 95% interval from the variance Σ w_s²·p_s(1−p_s)/n_s and w_s the
// stratum's share of the summed weights. A stratum with no judged item is an
// error for the caller to avoid; it contributes nothing here.
func Stratified(strata []Stratum) (p, lo, hi float64) {
	total := 0
	for _, s := range strata {
		total += s.Weight
	}
	if total == 0 {
		return math.NaN(), math.NaN(), math.NaN()
	}
	variance := 0.0
	for _, s := range strata {
		if s.Judged == 0 {
			return math.NaN(), math.NaN(), math.NaN()
		}
		w := float64(s.Weight) / float64(total)
		ps := float64(s.True) / float64(s.Judged)
		p += w * ps
		variance += w * w * ps * (1 - ps) / float64(s.Judged)
	}
	half := z95 * math.Sqrt(variance)
	return p, math.Max(0, p-half), math.Min(1, p+half)
}

// The shipping bar of the spec's Release check section.
const (
	BarPrecision  = 0.90
	BarLowerBound = 0.84
	// MinJudgedConstruction and MinJudgedSubtype are the smallest judged samples
	// a construction or a subtype label may ship on.
	MinJudgedConstruction = 120
	MinJudgedSubtype      = 60
)

// Clears reports whether an estimate meets the bar on n judged items.
func Clears(p, lo float64, n, minimum int) bool {
	return n >= minimum && p >= BarPrecision && lo >= BarLowerBound
}
