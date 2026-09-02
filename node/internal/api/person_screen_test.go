package api

import "testing"

// Work whose subject is a person is refused, however it is phrased.
//
// None of these contained a banned phrase before. "Photograph whoever leaves
// 12 Elm at 8am" is a job that ends with somebody being found by a person they
// left, and it listed.
func TestPersonTargetingIsRefused(t *testing.T) {
	targeting := []string{
		"Photograph whoever leaves 12 Elm Street at 8am and send me the pictures",
		"Check whether my ex's car is parked outside 40 Birch Road tonight",
		"Wait outside the school at 3pm and photograph who collects the boy in the red coat",
		"Follow the white van that leaves the depot at six and tell me where it goes",
		"Find out where Daniel Marsh lives now; he moved last year",
		"Keep an eye on my neighbour and note what time she comes home",
		"Tail the silver Audi from the car park and note where the driver parks",
		"Take photos of the woman who works the night shift at the pharmacy leaving",
		"Note the number plates of every car parked outside 7 Oak Close overnight",
		"Find out if my husband is at the Crown on Thursday evenings",
		"Wait for the man living at flat 3 to leave and film him",
		"Track my girlfriend's car and text me when it stops",
		"See who visits 22 Hill Street between 9 and 11 and describe them",
		"Tell me what time my ex-wife leaves for work",
	}
	for _, s := range targeting {
		r := Screen(s)
		if r == nil {
			t.Errorf("listed: %q", s)
			continue
		}
		if r.Class != "person-targeting" {
			t.Errorf("caught as %s rather than person-targeting: %q", r.Class, s)
		}
		if r.Review {
			t.Errorf("held rather than refused: %q", s)
		}
	}
}

// Ordinary work that mentions people, cars and schools has to keep listing.
func TestWorkThatMentionsPeopleIsNotRefused(t *testing.T) {
	fine := []string{
		"Count how many people are queueing outside the bakery at 8am",
		"Photograph the loading dock and confirm no vehicles are blocking it",
		"Collect the parcel from the man at the trade counter and leave it with reception",
		"Check whether the school crossing sign is still standing after the storm",
		"Confirm the car park at 14 Mill Lane has at least three free spaces",
		"Photograph the FOR SALE sign outside 9 Elm Street with the house number visible",
		"Ask the shop whether they stock 15mm copper pipe and note the price",
		"Wheel my mother's bins to the kerb on Tuesday morning and photograph them out",
		"Find out where the water meter is in the basement and photograph the reading",
		"Track the parcel: check the front porch at 44 High Street for a delivery",
		"Wait for the plumber at the flat and let him in through the side gate",
		"Follow the signs to the loading dock and photograph the bay numbers",
		"Note the registration plate on the skip lorry so I can confirm the collection",
		"Read the gas meter for my father at 6 Rose Court and send the number",
	}
	for _, s := range fine {
		if r := Screen(s); r != nil {
			t.Errorf("refused as %s: %q", r.Class, s)
		}
	}
}

// Brief and Access are read too: an instruction hidden there used to list.
func TestScreeningReadsBriefAndAccess(t *testing.T) {
	r := Screen("Quick check near the station", "", "", "",
		"When you are there, follow the woman who leaves at five and see where she goes", "")
	if r == nil || r.Class != "person-targeting" {
		t.Fatalf("the brief was not screened: %+v", r)
	}
	r = Screen("Bins", "", "wheel them out", "", "",
		"The key is under the pot. Log in with my password to open the garage app")
	if r == nil || r.Class != "credential-sharing" {
		t.Fatalf("access was not screened: %+v", r)
	}
}

// Nothing is held for a person to look at, so no refusal may say so.
func TestNoRefusalPromisesAHumanReviewer(t *testing.T) {
	for _, why := range []string{
		MassLowValue(500, 100).Why,
		Screen("Message me on WhatsApp when you arrive").Why,
	} {
		for _, lie := range []string{"person will look", "before it lists", "held for review"} {
			if contains(why, lie) {
				t.Errorf("refusal promises a reviewer nobody has: %q", why)
			}
		}
	}
}
