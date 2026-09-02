package api

import (
	"fmt"
	"regexp"
	"strings"
)

// Work this exchange will not carry.
//
// Written after reading what happened to the first marketplace of this shape.
// Researchers cataloguing rentahuman.ai found six abuse classes being bought
// openly: fake accounts on third-party services at $12-15 each, people paid
// $60/hour to impersonate professionals in job interviews, one-time passcodes
// solicited from workers, engagement farming across hundreds of accounts,
// referral fraud with real KYC, and reconnaissance dispatched by closed-loop
// pipelines with no human in them. A bounty asking for help defeating
// two-factor authentication drew 79 applicants.
//
// The workers were not the problem. Most had no idea what they were part of:
// creating an account for somebody is a five-minute errand until it turns out
// to be a money-mule pipeline, and the person who did it is the one whose name
// is on it. An exchange that dispatches physical acts without asking what they
// are for is a laundering service with a job board attached.
//
// The published analysis found seven keyword rules caught 51 of 52 abusive
// listings with a 0.3% false-positive rate. That is the cheapest safety
// mechanism available here and it did not exist.

// Refusal explains why a job cannot be listed.
type Refusal struct {
	// Class is the abuse pattern matched, for the record and for review.
	Class string
	// Why is what the buyer's agent is told.
	Why string
	// Review marks a class with honest readings, so the refusal can say what
	// would list instead. Nothing is held for a person to look at: there is
	// no review queue, and a message promising one would be a lie. The job
	// is refused either way.
	Review bool
}

func (r Refusal) Error() string { return r.Why }

type screenRule struct {
	class   string
	pattern *regexp.Regexp
	why     string
	review  bool
}

// The rules. Deliberately few and deliberately blunt.
//
// Each one names a class of harm rather than a keyword, because a list of
// banned words is a list somebody edits around. Where an honest reading exists
// the refusal says what to change — a locksmith opening a door and a burglar
// opening a door describe themselves the same way, and the difference is a
// licence, not a phrase — but it is still a refusal: nobody is queued to look.
var screenRules = []screenRule{
	{
		class:   "account-creation",
		pattern: regexp.MustCompile(`(?i)\b(create|open|register|sign[ -]?up for|set[ -]?up)\b[^.]{0,40}\b(account|profile|wallet)\b`),
		why: "this exchange does not carry work that involves opening accounts " +
			"in somebody else's name or on somebody else's behalf. That is how " +
			"money-mule and identity pipelines are staffed, and the person who " +
			"opens the account is the one who carries it.",
	},
	{
		class:   "one-time-code",
		pattern: regexp.MustCompile(`(?i)\b(otp|one[- ]time (code|password|passcode)|2fa|two[- ]factor|verification code|sms code|auth code)\b`),
		why: "this exchange does not carry work involving one-time codes or " +
			"two-factor authentication. There is no honest version of asking a " +
			"stranger to read you a passcode.",
	},
	{
		class:   "impersonation",
		pattern: regexp.MustCompile(`(?i)\b(pretend to be|impersonat|pose as|act as (?:me|my|an? (?:employee|manager|engineer))|on my behalf in (?:an? )?(?:interview|call))\b`),
		why: "this exchange does not carry work that involves representing " +
			"yourself as somebody else. Interview and identity impersonation is " +
			"the single best-paid abuse on marketplaces of this kind.",
	},
	{
		class:   "engagement-farming",
		pattern: regexp.MustCompile(`(?i)\b(follow|like|upvote|retweet|review|comment on|subscribe to)\b[^.]{0,30}\b(account|post|profile|page|listing|video|product)\b`),
		why: "this exchange does not carry paid engagement, reviews or " +
			"followers. Buying authentic-looking human activity is exactly what " +
			"platform bot-detection cannot see, which is why it is bought.",
	},
	{
		class:   "referral-kyc",
		pattern: regexp.MustCompile(`(?i)\b(referral (link|code|bonus)|refer a friend|use my (?:code|link))\b`),
		why: "this exchange does not carry referral or signup-bonus work. It " +
			"pairs with identity verification to turn real people into farmed " +
			"accounts.",
	},
	{
		class:   "credential-sharing",
		pattern: regexp.MustCompile(`(?i)\b(my (?:password|login|credentials)|log ?in (?:as|with) (?:my|these)|share (?:your|the) (?:password|credentials))\b`),
		why: "this exchange does not carry work that passes login credentials " +
			"between people, in either direction.",
	},
	{
		// A task whose subject is a person. Following somebody, waiting for
		// them, photographing whoever comes and goes, finding out where they
		// live, tailing a car: the honest readings are rare and the dishonest
		// ones end with somebody being found by a person they left. The rule
		// looks for a surveillance verb aimed at a person or a vehicle, for a
		// question about where a named person lives or works, and for
		// children at a school. It is refused, not held: there is no queue of
		// people reviewing these, and saying there was would be a lie.
		class:   "person-targeting",
		pattern: personTargeting,
		why: "this exchange does not carry work whose subject is a person: " +
			"following, watching or photographing somebody, finding out where " +
			"they live or work, or tracking a vehicle. Refused.",
	},
	{
		class:   "off-platform",
		pattern: regexp.MustCompile(`(?i)\b(whats ?app|telegram|signal me|dm me|text me at|email me at|discord|pay(?:ment)? outside|off[- ]platform)\b`),
		why: "arrangements moved off the exchange lose the escrow, the evidence " +
			"and the record — which is usually the point of moving them. Keep " +
			"contact and payment here.",
		review: true,
	},
}

// personTargeting is assembled from parts so each can be read on its own.
var personTargeting = regexp.MustCompile(`(?i)` + strings.Join([]string{
	// Follow, tail, watch or track a person or a vehicle.
	`\b(?:follow|tail|trail|shadow|track|stalk|surveil|spy on|keep (?:an )?eye on|keep watch on|keep tabs on)\b[^.]{0,40}\b(?:` + personNoun + `|car|vehicle|truck|van|plates?)\b`,
	// Photograph, film or wait for whoever comes and goes, or a particular
	// person.
	`\b(?:photograph|film|record|video|snap|take (?:a |some )?(?:photos?|pictures?|videos?) of|watch|wait for|wait outside for|look out for|see|note|log|identify)\b[^.]{0,40}\b(?:whoever|anyone who|anybody who|everyone who|everybody who|people who|who (?:comes|goes|leaves|arrives|enters|exits|visits|lives|collects|picks up|answers|drops off)|the (?:man|woman|person|guy|girl|couple|family) (?:who|that|at|in|living|staying)|(?:my|his|her|their) (?:` + personNoun + `))\b`,
	// Where a person lives, works or sleeps.
	`\bwhere\s+(?:\S+\s+){1,4}?(?:lives?|works?|sleeps?|is staying|stays|is living)\b`,
	// Whether somebody's partner, ex or child is somewhere.
	`\b(?:whether|if|when|what time)\s+(?:my|his|her|their)\s+(?:` + personNoun + `)(?:'s)?\b[^.]{0,30}\b(?:is|was|are|were|comes|goes|leaves|arrives|gets|home|there|in|out|at|parked)\b`,
	// Anything at all about an ex.
	`\b(?:my|his|her|their)\s+ex(?:-?(?:wife|husband|partner|boyfriend|girlfriend))?\b`,
	// A car parked somewhere and who it belongs to.
	`\b(?:licen[cs]e|number|reg(?:istration)?) plates?\b[^.]{0,40}\b(?:parked|who|whose|owner|visits?|visiting|comes|outside|at)\b`,
	`\bwhose car\b|\bwho (?:is|was) (?:parked|visiting|staying|sleeping)\b`,
	// Children at a school.
	`\b(?:outside|near|at|by|from)\s+(?:the |a |their |his |her |my )?(?:school|playground|nursery|daycare|kindergarten|preschool)\b[^.]{0,40}\b(?:kids?|child|children|pupils?|students?|boys?|girls?|son|daughter|pick-?up|collect(?:s|ed|ing)?|who)\b`,
}, "|"))

// personNoun is who a surveillance verb might be aimed at.
const personNoun = `person|people|man|woman|guy|girl|boy|lady|kid|kids|child|children|` +
	`daughter|son|wife|husband|ex|boyfriend|girlfriend|partner|spouse|neighbou?r|` +
	`tenant|landlord|employee|colleague|co-?worker|boss|friend|brother|sister|` +
	`mother|father|mum|mom|dad|resident|occupant`

// Screen inspects everything a buyer wrote for work this exchange refuses.
//
// Applied to the whole of it: predicate, detail and instructions together,
// because the interesting part is rarely in the title.
func Screen(parts ...string) *Refusal {
	text := strings.Join(parts, "\n")
	if strings.TrimSpace(text) == "" {
		return nil
	}
	for _, r := range screenRules {
		if r.pattern.MatchString(text) {
			return &Refusal{Class: r.class, Why: r.why, Review: r.review}
		}
	}
	return nil
}

// ScreenAll returns every class a job matches, for the audit record. Screen
// stops at the first because the buyer only needs one reason.
func ScreenAll(parts ...string) []string {
	text := strings.Join(parts, "\n")
	var out []string
	for _, r := range screenRules {
		if r.pattern.MatchString(text) {
			out = append(out, r.class)
		}
	}
	return out
}

// MassLowValue flags the shape the researchers found under the abusive
// listings: many people wanted, very little paid to each.
//
// Not a phrase, so no wording avoids it. Honest work at this shape exists —
// leafleting, queueing — so it is held for review rather than refused.
func MassLowValue(slots int, payMinor int64) *Refusal {
	const manySlots = 20
	const lowPay = 300
	if slots >= manySlots && payMinor > 0 && payMinor <= lowPay {
		return &Refusal{
			Class:  "mass-low-value",
			Review: true,
			Why: fmt.Sprintf(
				"%d people at %d minor units each is the shape engagement "+
					"farming takes, and it is refused as posted. Fewer people at "+
					"a real rate lists without trouble.",
				slots, payMinor),
		}
	}
	return nil
}
