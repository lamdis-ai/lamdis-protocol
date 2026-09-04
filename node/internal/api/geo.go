package api

import "math"

// Distance, in the units the people using this actually think in.
//
// Positions are stored as integer degrees times 1e7 everywhere, because a
// float in a struct that also carries money is an invitation to put a float in
// the money. Conversion happens here and nowhere else.

// E7 converts decimal degrees to the stored integer form.
func E7(deg float64) int64 { return int64(deg * 1e7) }

// Deg converts back.
func Deg(e7 int64) float64 { return float64(e7) / 1e7 }

// MilesBetween is the great-circle distance between two stored positions.
func MilesBetween(latE7A, lonE7A, latE7B, lonE7B int64) float64 {
	const earthMiles = 3958.8
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	lat1, lon1 := Deg(latE7A), Deg(lonE7A)
	lat2, lon2 := Deg(latE7B), Deg(lonE7B)
	dLat, dLon := rad(lat2-lat1), rad(lon2-lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthMiles * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// HasPosition reports whether a stored position is set at all.
//
// Zero is a real coordinate in the Gulf of Guinea, so it cannot mean "unknown"
// — but a job or an operator at exactly 0,0 is a data error rather than a
// customer, and treating it as unset is the safer reading.
func HasPosition(latE7, lonE7 int64) bool { return latE7 != 0 || lonE7 != 0 }

// InRange reports whether a job is close enough for an operator to take.
//
// Unknown positions do not exclude: a job with no coordinates is one the buyer
// described in words, and an operator who has not shared a location has not
// asked to be filtered. Refusing either would hide work from people who could
// do it, which is the more expensive mistake.
func InRange(jobLatE7, jobLonE7, opLatE7, opLonE7 int64, rangeMiles int) bool {
	if rangeMiles <= 0 {
		return true
	}
	if !HasPosition(jobLatE7, jobLonE7) || !HasPosition(opLatE7, opLonE7) {
		return true
	}
	return MilesBetween(jobLatE7, jobLonE7, opLatE7, opLonE7) <= float64(rangeMiles)
}

// CoarseE7 rounds a stored position to two decimal places of a degree —
// roughly a kilometre — which is the only precision anything public here
// publishes, and the only precision anything public here stores.
//
// A hundredth of a degree is exactly 100000 in the stored form, so the
// rounding is done in that unit rather than by multiplying a rounded float
// back up: 42.33 is not representable, and int64(42.33*1e7) is 423299999.
func CoarseE7(e7 int64) int64 { return int64(math.Round(Deg(e7)*100)) * 100000 }
