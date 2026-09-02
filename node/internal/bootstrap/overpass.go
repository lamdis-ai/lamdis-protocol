package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// Real places, from public data.
//
// OpenStreetMap's Overpass API answers "what businesses are within a
// kilometre of here" with no key and no account. Its usage policy asks for a
// User-Agent that says who is calling, a modest rate, and no more than what
// you need — so the loop makes one query per cluster per cycle, names itself,
// and asks for a bounded number of results.

// DefaultOverpassURL is the public instance. Deployments that run their own
// set LAMDIS_OVERPASS_URL.
const DefaultOverpassURL = "https://overpass-api.de/api/interpreter"

// userAgent names the caller, as the Overpass usage policy asks.
const userAgent = "lamdis-exchange-bootstrap/0.1 (+https://exchange.lamdis.ai)"

// maxPlaces bounds one query. Three jobs a cycle need a few dozen candidates
// after filtering, not a whole city.
const maxPlaces = 80

// Place is one business or public venue OpenStreetMap knows about.
type Place struct {
	// ID is the OSM element, e.g. "node/123" or "way/456". Stable across
	// queries, which is what the dedupe set keys on.
	ID   string `json:"id"`
	Name string `json:"name"`
	// Category is the tag that made it eligible: "shop=bakery", "amenity=cafe".
	Category     string `json:"category"`
	HouseNumber  string `json:"house_number,omitempty"`
	Street       string `json:"street,omitempty"`
	City         string `json:"city,omitempty"`
	LatE7, LonE7 int64
	// Tags are kept only until eligibility is decided; never published.
	Tags map[string]string `json:"-"`
}

// PlaceSource finds places near a point. The Overpass client is one; a test
// fixture is another.
type PlaceSource interface {
	Nearby(ctx context.Context, latE7, lonE7 int64, radiusM int) ([]Place, error)
}

// Overpass queries an Overpass API instance over HTTP.
type Overpass struct {
	URL    string
	Client *http.Client
}

// NewOverpass builds a client. An empty URL means the public instance.
func NewOverpass(url string) *Overpass {
	if url == "" {
		url = DefaultOverpassURL
	}
	return &Overpass{URL: url, Client: &http.Client{Timeout: 45 * time.Second}}
}

// commercialKeys are the tags that mark a place as a business or public venue
// rather than somewhere somebody lives.
var commercialKeys = []string{"shop", "amenity", "office", "craft", "tourism"}

// Nearby asks for named commercial places around a point.
func (o *Overpass) Nearby(ctx context.Context, latE7, lonE7 int64, radiusM int) ([]Place, error) {
	q := overpassQuery(latE7, lonE7, radiusM)
	form := url.Values{"data": {q}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.URL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	res, err := o.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("overpass: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("overpass: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("overpass: status %d", res.StatusCode)
	}
	return ParseOverpass(body)
}

// overpassQuery asks for named nodes, ways and relations carrying any
// commercial tag inside the circle. Residential filtering happens on the
// result, because Overpass cannot express "not somebody's home" any better
// than we can.
func overpassQuery(latE7, lonE7 int64, radiusM int) string {
	var b strings.Builder
	b.WriteString("[out:json][timeout:25];(")
	for _, k := range commercialKeys {
		fmt.Fprintf(&b, "nwr(around:%d,%.6f,%.6f)[%q][name];",
			radiusM, api.Deg(latE7), api.Deg(lonE7), k)
	}
	fmt.Fprintf(&b, ");out center %d;", maxPlaces)
	return b.String()
}

// ParseOverpass reads an Overpass JSON response into places. Exported so a
// canned response can be used without the network.
func ParseOverpass(data []byte) ([]Place, error) {
	var res struct {
		Elements []struct {
			Type   string                      `json:"type"`
			ID     int64                       `json:"id"`
			Lat    float64                     `json:"lat"`
			Lon    float64                     `json:"lon"`
			Center *struct{ Lat, Lon float64 } `json:"center"`
			Tags   map[string]string           `json:"tags"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("overpass: unreadable response: %w", err)
	}
	var out []Place
	for _, e := range res.Elements {
		lat, lon := e.Lat, e.Lon
		if e.Center != nil {
			lat, lon = e.Center.Lat, e.Center.Lon
		}
		if lat == 0 && lon == 0 {
			continue
		}
		p := Place{
			ID:          e.Type + "/" + fmt.Sprint(e.ID),
			Name:        strings.TrimSpace(e.Tags["name"]),
			HouseNumber: e.Tags["addr:housenumber"],
			Street:      e.Tags["addr:street"],
			City:        e.Tags["addr:city"],
			LatE7:       api.E7(lat), LonE7: api.E7(lon),
			Tags: e.Tags,
		}
		for _, k := range commercialKeys {
			if v := e.Tags[k]; v != "" {
				p.Category = k + "=" + v
				break
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// residentialBuildings are building tags that mean somebody lives there.
var residentialBuildings = map[string]bool{
	"house": true, "residential": true, "apartments": true, "detached": true,
	"semidetached_house": true, "terrace": true, "bungalow": true, "dormitory": true,
	"cabin": true, "static_caravan": true, "farm": true, "hut": true,
}

// homelikeAmenities house people rather than serve customers.
var homelikeAmenities = map[string]bool{
	"shelter": true, "social_facility": true, "nursing_home": true,
	"childcare": true, "kindergarten": true, "refugee_site": true,
}

// publicTourism are the tourism values that are venues, not lodgings somebody
// lives in. Anything else under tourism is refused.
var publicTourism = map[string]bool{
	"attraction": true, "museum": true, "gallery": true, "information": true,
	"viewpoint": true, "artwork": true, "zoo": true, "theme_park": true, "aquarium": true,
}

// Eligible reports whether a place may be the subject of a house job.
//
// Only businesses and public venues, never homes. A shop tag on a building
// tagged as a house is somebody's home business, and the loop must not send
// a stranger to photograph it.
func Eligible(p Place) bool {
	if p.Name == "" || p.Category == "" || !api.HasPosition(p.LatE7, p.LonE7) {
		return false
	}
	if residentialBuildings[p.Tags["building"]] {
		return false
	}
	if homelikeAmenities[p.Tags["amenity"]] {
		return false
	}
	if t := p.Tags["tourism"]; t != "" && !publicTourism[t] && p.Tags["shop"] == "" &&
		p.Tags["amenity"] == "" && p.Tags["office"] == "" && p.Tags["craft"] == "" {
		return false
	}
	return true
}

// HasAddress reports whether the place has a number and a street, which the
// vacancy and corner shapes need.
func (p Place) HasAddress() bool { return p.HouseNumber != "" && p.Street != "" }

// Address is the street address, or empty.
func (p Place) Address() string {
	if !p.HasAddress() {
		return ""
	}
	return p.HouseNumber + " " + p.Street
}
