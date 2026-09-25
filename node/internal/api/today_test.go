package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The home screen gathers what is already in the record: schedules, paused
// ones included, and the name the person gave their agent.
func TestTodayGathersSchedulesAndTheAgentsName(t *testing.T) {
	h, _, _, handler := testHost(t)
	h.Guests = true
	w := call(handler, "POST", "/app/api/start", "", "")
	var started struct {
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &started)
	tok := started.Token
	th := firstThread(t, handler, tok)

	if w := call(handler, "POST", "/app/api/agent/config", tok, `{"name":"@Juniper"}`); w.Code != http.StatusOK || strings.Contains(w.Body.String(), "error") {
		t.Fatalf("naming the agent: %d %s", w.Code, w.Body.String())
	}
	if w := call(handler, "POST", "/app/api/agent/config", tok, `{"name":"<script>"}`); !strings.Contains(w.Body.String(), "error") {
		t.Fatalf("a name with markup in it was accepted: %s", w.Body.String())
	}
	brief := `{"text":"","on_new_entry":"others","rhythms":[` +
		`{"name":"morning","at":"07:30","zone":"UTC","prompt":"what changed?"},` +
		`{"name":"sunday","at":"09:00","zone":"UTC","prompt":"digest","paused":true}]}`
	if w := call(handler, "POST", "/app/api/thread/"+th+"/brief", tok, brief); w.Code != http.StatusOK {
		t.Fatalf("brief: %d %s", w.Code, w.Body.String())
	}

	// A channel's own full-auto setting survives the round trip.
	if w := call(handler, "POST", "/app/api/thread/"+th+"/brief", tok, strings.Replace(brief, `{"text":""`, `{"text":"","autonomy":"auto"`, 1)); w.Code != http.StatusOK {
		t.Fatalf("brief with autonomy: %d %s", w.Code, w.Body.String())
	}
	if w := call(handler, "GET", "/app/api/threads", tok, ""); !strings.Contains(w.Body.String(), `"full_auto":true`) {
		t.Fatalf("full auto was not kept: %s", w.Body.String())
	}
	w = call(handler, "GET", "/app/api/today", tok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("today: %d %s", w.Code, w.Body.String())
	}
	var got struct {
		Agent     string `json:"agent"`
		Schedules []struct {
			Thread string `json:"thread"`
			Index  int    `json:"index"`
			At     string `json:"at"`
			Paused bool   `json:"paused"`
		} `json:"schedules"`
		Watching  []struct{ Thread string } `json:"watching"`
		Decisions []any                     `json:"decisions"`
		Runs      []any                     `json:"runs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Agent != "Juniper" {
		t.Fatalf("the agent's name came back as %q", got.Agent)
	}
	if len(got.Schedules) != 2 || got.Schedules[0].At != "07:30" || got.Schedules[0].Paused || !got.Schedules[1].Paused || got.Schedules[1].Index != 1 {
		t.Fatalf("schedules: %+v", got.Schedules)
	}
	if len(got.Watching) != 1 || got.Watching[0].Thread != th {
		t.Fatalf("watching: %+v", got.Watching)
	}
	if got.Decisions == nil || got.Runs == nil {
		t.Fatal("empty lists must be lists, not null, so the page can count them")
	}
}
