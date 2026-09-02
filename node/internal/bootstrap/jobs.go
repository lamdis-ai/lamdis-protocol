package bootstrap

import (
	"fmt"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// The jobs the house posts.
//
// Four shapes, all observations of a business from the public way, all
// answerable with one photograph that has a sign or a house number in frame.
// They are chosen because the answers are worth something to somebody who is
// not here: whether a business is open and signed as listed, what its hours
// are, whether a storefront is empty, and whether an address is marked the
// way the map says. That is the data this loop exists to produce.

// Shape is one kind of house job.
type Shape string

const (
	ShapeOpenSignage Shape = "open-signage"
	ShapeHours       Shape = "hours"
	ShapeVacant      Shape = "vacant"
	ShapeCorner      Shape = "corner"
)

// shapes is the rotation order.
var shapes = []Shape{ShapeOpenSignage, ShapeHours, ShapeVacant, ShapeCorner}

// The pay ladder, in one place.
//
// Observe fees run from four to eight dollars by how much the shape asks for;
// a wasted trip pays two. Small on purpose — this is seed demand, and its job
// is to give the first operators something real to do near where they are,
// not to set a price for the market.
const (
	PayOpenSignageMinor int64 = 400
	PayHoursMinor       int64 = 500
	PayVacantMinor      int64 = 600
	PayCornerMinor      int64 = 800
	AttemptMinor        int64 = 200
	// JobTTL is how long a house job stays open.
	JobTTL = 48 * time.Hour
	// JobRadiusM is the geofence: the photograph must come from within it.
	JobRadiusM int64 = 150
	// JobTier is the verification standard. V2 ties the photograph to the
	// place, which is the whole value of the finding.
	JobTier = "V2"
)

// HousePrincipal is the account the loop spends from. It is the only account
// the loop can move money for.
const HousePrincipal = "house:bootstrap"

// payFor is what a shape pays.
func payFor(s Shape) int64 {
	switch s {
	case ShapeHours:
		return PayHoursMinor
	case ShapeVacant:
		return PayVacantMinor
	case ShapeCorner:
		return PayCornerMinor
	default:
		return PayOpenSignageMinor
	}
}

// needsAddress reports whether a shape only makes sense with a street address.
func needsAddress(s Shape) bool { return s == ShapeVacant || s == ShapeCorner }

// shapeFor picks the nth shape in rotation, falling back to one the place can
// support.
func shapeFor(p Place, n int) Shape {
	s := shapes[n%len(shapes)]
	if needsAddress(s) && !p.HasAddress() {
		s = shapes[n%2]
	}
	return s
}

// detail is what the job is for, said on the board. Software wrote it and
// the board says so separately; this says why.
const detail = "Posted by the exchange itself, not a customer. The answer " +
	"goes into a public dataset of verified facts about storefronts — " +
	"whether a place is open and signed as mapped, its posted hours, whether " +
	"it is vacant. Photograph from the public way only. Do not enter, and do " +
	"not photograph people."

// Compose builds the listing for one place and shape.
func Compose(job string, p Place, s Shape, area string, now time.Time) *api.Listing {
	where := p.Name
	if a := p.Address(); a != "" {
		where += ", " + a
	}
	if p.City != "" {
		where += ", " + p.City
	}
	if area == "" {
		area = p.City
	}
	l := &api.Listing{
		Job: job, Kind: api.KindObserve,
		Detail: detail,
		Where:  where, Area: area,
		LatE7: p.LatE7, LonE7: p.LonE7, RadiusM: JobRadiusM,
		PayMinor: payFor(s), AttemptMinor: AttemptMinor, Currency: "USD",
		Slots: 1, Tier: JobTier,
		PostedByAgent: true,
		Expires:       now.Add(JobTTL), Posted: now,
		Owner: HousePrincipal,
	}
	num := p.HouseNumber
	if num == "" {
		num = "the house number"
	}
	switch s {
	case ShapeHours:
		l.Title = fmt.Sprintf("Photograph the posted opening hours on the door of %s", p.Name)
		l.Instructions = "Go to the entrance. Photograph the posted hours so " +
			"every line is readable, with the code in frame. If there are no " +
			"hours posted, photograph the door and say so."
		l.Deliverable = fmt.Sprintf("One photo of the posted hours at the door of %s, "+
			"legible, with the business sign or %s visible and the code in frame.",
			p.Name, num)
	case ShapeVacant:
		l.Title = fmt.Sprintf("Is the storefront at %s vacant?", p.Address())
		l.Instructions = "Photograph the storefront from the sidewalk. Papered " +
			"windows, an empty interior, a for-lease sign or a removed fascia " +
			"all count as vacant; a trading business does not."
		l.Deliverable = fmt.Sprintf("One photo of the storefront at %s with the "+
			"house number %s in frame and the code legible.", p.Address(), p.HouseNumber)
	case ShapeCorner:
		l.Title = fmt.Sprintf("Photograph the house number %s and the street sign for %s at this corner",
			p.HouseNumber, p.Street)
		l.Instructions = "Stand where both the number on the building and the " +
			"nearest street sign are in one frame if you can; two photos if " +
			"not. The code must be legible in at least one."
		l.Deliverable = fmt.Sprintf("Photos showing the house number %s and the "+
			"street sign for %s, each legible, with the code in frame.",
			p.HouseNumber, p.Street)
	default:
		l.Title = fmt.Sprintf("Is %s open right now, and does the signage match the name?", p.Name)
		l.Instructions = "Photograph the front of the business from the public " +
			"way with the sign in frame. Report whether it is trading at the " +
			"time of the photo and whether the sign reads as the name given."
		l.Deliverable = fmt.Sprintf("One photo of the frontage with the sign for "+
			"%s and %s visible, and the code in frame.", p.Name, num)
	}
	return l
}
