package exchange

import (
	"os"
	"strings"
	"testing"
)

// The routes that describe how an account spends must accept what the route
// that actually spends already accepts.
//
// POST /v1/tasks has always taken a signed principal — an integration holding
// its own keypair. Every buy-side route added around it took only an agent key
// or a Cognito-verified person, so a company could post a job with its own key
// and could not name the vendor to send it to, add the site it happens at, or
// ask what anything cost.
//
// Walking the journey over HTTP was how this surfaced: six of seven steps
// answered 401. Reading the code, it looked finished.
func TestBuyerRoutesAcceptTheSameCredentialAsPosting(t *testing.T) {
	src, err := readFile("review.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "AuthenticatePrincipal") {
		t.Fatal("withBuyer does not accept a signed principal, so an " +
			"integration that can post a job cannot describe how to spend")
	}
	// All three, and the refusal has to name them or nobody knows what to send.
	for _, want := range []string{"AuthenticateAgent", "Workers.Authenticate", "AuthenticatePrincipal"} {
		if !strings.Contains(src, want) {
			t.Errorf("withBuyer does not consider %s", want)
		}
	}
	if !strings.Contains(src, "sign the request with your principal key") {
		t.Error("the refusal does not tell an integration what would work")
	}
}

// Enrolment alone is not verification, and the difference must stay visible.
//
// Enroll sets Enrolled and never Verified. That is correct — a keypair is not
// an identity — but it means anything gated on Verified is unreachable to an
// integration, which is how the whole enterprise surface ended up behind a
// personal email code.
func TestEnrolmentIsNotVerification(t *testing.T) {
	src, err := readFile("../api/worker.go")
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(src, "func (ws *Workers) Enroll(")
	if i < 0 {
		t.Fatal("Enroll is gone")
	}
	body := src[i : i+700]
	if strings.Contains(body, "Verified: true") {
		t.Error("enrolling with a keypair now marks somebody verified; " +
			"verification is supposed to mean an identity provider vouched")
	}
	if !strings.Contains(body, "Enrolled: true") {
		t.Error("enrolment no longer records itself")
	}
}

func readFile(rel string) (string, error) {
	b, err := os.ReadFile(rel)
	return string(b), err
}

// The console funds an account with the person's session token. The route it
// calls must accept that credential, or the only way money enters the exchange
// answers 401 to every signed-in buyer.
//
// This surfaced by reading the console: addFunds() sends "Authorization:
// Bearer <session>" to POST /v1/balance/topup, and the route was registered
// withAgent, which reads only X-Lamdis-Key. handleAuthFailure then cleared the
// session and sent a correctly signed-in buyer back to /signin.
func TestFundingRoutesAcceptTheConsoleSession(t *testing.T) {
	src, err := readFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/v1/balance/topup", "/v1/balance/withdraw"} {
		i := strings.Index(src, route)
		if i < 0 {
			t.Fatalf("%s is no longer registered", route)
		}
		line := src[i:]
		if j := strings.IndexByte(line, '\n'); j >= 0 {
			line = line[:j]
		}
		if !strings.Contains(line, "withBuyer") {
			t.Errorf("%s is registered %q; the console sends a session token, "+
				"which only withBuyer accepts", route, line)
		}
	}
}
