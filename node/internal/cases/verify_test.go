package cases

import "testing"

func TestContains(t *testing.T) {
	text2 := "or 27 kg/m2 or greater with at least one weight-\nrelated comorbid condition"
	for _, q := range []string{"at least one weight-", "at least one weight-related comorbid condition", "at least one weightrelated"} {
		if !Contains(text2, q) {
			t.Errorf("line-break hyphen: %q not found", q)
		}
	}
	if Contains(text2, "at least two weight-related") {
		t.Error("a different word matched")
	}
	text := "We reviewed the request for Zepbound (tirzepatide).\nThe re-\nquest is not medically necessary because the member’s BMI is below 30."
	cases := []struct {
		q    string
		want bool
	}{
		{"not medically necessary because the member's BMI is below 30", true},
		{"The request is not medically necessary", true},
		{"“Zepbound (tirzepatide)”", true},
		{"medically necessary because the member's BMI is below 35", false},
		{"is", false},
		{"We approved the request", false},
		{"is below 30", true},
		{"BMI is below 35", false},
	}
	for _, c := range cases {
		if got := Contains(text, c.q); got != c.want {
			t.Errorf("Contains(%q) = %v, want %v", c.q, got, c.want)
		}
	}
}
