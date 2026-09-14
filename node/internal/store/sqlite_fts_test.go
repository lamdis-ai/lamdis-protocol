package store

import "testing"

// A question is the shape an agent asks in. The previous builder AND-ed every
// word, so one function word nobody stored made the whole query miss.
func TestFTSQuerySurvivesASentence(t *testing.T) {
	got := ftsQuery("does the exchange hold the money on the usdc rail")
	want := `"exchange" OR "hold" OR "money" OR "usdc" OR "rail"`
	if got != want {
		t.Errorf("ftsQuery:\n got %s\nwant %s", got, want)
	}
}

// Keyword queries must not regress into something looser than they were.
func TestFTSQueryKeepsKeywords(t *testing.T) {
	if got, want := ftsQuery("watch-only rail"), `"watch-only" OR "rail"`; got != want {
		t.Errorf("ftsQuery = %s, want %s", got, want)
	}
}

// An empty MATCH is a syntax error, not an empty result set, so a query made
// only of function words has to still produce something legal.
func TestFTSQueryAllStopwords(t *testing.T) {
	if got := ftsQuery("what is it"); got == "" {
		t.Fatal("ftsQuery returned an empty MATCH expression")
	}
}

// A quote in the query must not break out of the quoted term.
func TestFTSQueryEscapesQuotes(t *testing.T) {
	if got, want := ftsQuery(`clause "7"`), `"clause" OR """7"""`; got != want {
		t.Errorf("ftsQuery = %s, want %s", got, want)
	}
}
