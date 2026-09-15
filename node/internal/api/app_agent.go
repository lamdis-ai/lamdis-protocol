package api

// The agent's routes. Chat writes the question and the answer into the
// thread; a brief sets standing instructions; a decision is how the person
// answers the agent; the agent endpoint is who the agent is and what it did
// today. Nothing here can grant access: that stays a person-signed control
// entry, written elsewhere.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
)

const noModel = "No model is configured, so your agent cannot answer yet. " +
	"Add an OpenRouter key in Settings (keys are at openrouter.ai/keys), or point at a local model server. Everything else works without it."

// runCtx detaches a run from the request so a closed tab does not abandon a
// half-written run; the runner has its own deadline.
func runCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Minute)
}

func (a *App) agentReady() (string, bool) {
	if a.Runner == nil || !a.Runner.Ready() {
		return noModel, false
	}
	if a.agentRevoked {
		return "The agent's key was revoked. Restart the node to mint a new one.", false
	}
	return "", true
}

// personAppend writes one entry signed by the person.
func (a *App) personAppend(ctx context.Context, thread string, d protolog.Draft) (*protolog.Entry, error) {
	tl, err := a.Store.Thread(ctx, thread)
	if err != nil {
		return nil, err
	}
	author, err := protolog.NewAuthor(tl, a.Key)
	if err != nil {
		return nil, err
	}
	e, err := author.Append(d)
	if err != nil {
		return nil, err
	}
	if err := a.Store.AppendEntries(ctx, []*protolog.Entry{e}); err != nil {
		return nil, err
	}
	return e, nil
}

func (a *App) handleChat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Thread string `json:"thread"`
		Text   string `json:"text"`
		Scope  string `json:"scope"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in) != nil || strings.TrimSpace(in.Text) == "" || in.Thread == "" {
		http.Error(w, "thread and text are required", http.StatusBadRequest)
		return
	}
	if msg, ok := a.agentReady(); !ok {
		writeJSON(w, map[string]any{"error": msg})
		return
	}
	ctx, cancel := runCtx()
	defer cancel()
	q, err := a.personAppend(ctx, in.Thread, protolog.Draft{Kind: agent.KindQuestion, Lane: protolog.LaneContent,
		Body: map[string]any{"text": strings.TrimSpace(in.Text), "scope": in.Scope}})
	if err != nil {
		http.Error(w, "no such thread", http.StatusNotFound)
		return
	}
	res := a.Runner.Run(ctx, agent.Trigger{Kind: agent.TriggerChat, Thread: in.Thread, Entry: q.ID})
	if a.Scheduler != nil {
		a.Scheduler.Consume(ctx, in.Thread)
	}
	writeJSON(w, map[string]any{"question": q.ID, "answer": res.Answer, "answer_id": res.AnswerID,
		"run": res.RunID, "outcome": res.Outcome, "error": res.Error, "waiting": res.Outcome == "waiting",
		"model": a.Model})
}

func (a *App) handleBriefGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tl, err := a.Store.Thread(r.Context(), id)
	if err != nil {
		http.Error(w, "no such thread", http.StatusNotFound)
		return
	}
	b, has := agent.LoadBrief(tl, a.Self)
	writeJSON(w, map[string]any{"brief": b, "has": has, "reach": a.reach()})
}

// reach is what the node can offer a thread: the domains and tools the
// person configured. The brief picks from it.
func (a *App) reach() map[string]any {
	cfg, _ := agent.LoadConfig(a.DataDir)
	type toolOut struct {
		Name    string   `json:"name"`
		Tools   []string `json:"tools"`
		Confirm []string `json:"confirm"`
	}
	tools := []toolOut{}
	for _, t := range cfg.Tools {
		tools = append(tools, toolOut{Name: t.Name, Tools: t.Allow, Confirm: t.Confirm})
	}
	_, modelName := a.Runner.ModelFor(cfg)
	choices := a.Runner.Choices(cfg)
	return map[string]any{"allow_domains": cfg.AllowDomains, "tools": tools,
		"model_choices": choices, "own_key": cfg.OpenRouterKey != "" || cfg.ModelURLKey != "",
		"budget":           map[string]any{"runs": cfg.MaxRunsPerDay, "tokens": cfg.MaxTokensPerDay, "fetches": cfg.MaxFetchesPerDay},
		"max_runs_per_day": cfg.MaxRunsPerDay, "max_fetches_per_day": cfg.MaxFetchesPerDay,
		"max_tokens_per_day": cfg.MaxTokensPerDay, "config_path": a.DataDir + "/agent.json",
		"auto_web": cfg.AutoWeb, "may_hold_secrets": a.mayHoldSecrets(),
		"model": modelName, "model_url": cfg.ModelURL, "has_key": cfg.OpenRouterKey != "" || os.Getenv("LAMDIS_OPENROUTER_KEY") != "",
		"key_from_env": os.Getenv("LAMDIS_OPENROUTER_KEY") != "", "has_url_key": cfg.ModelURLKey != ""}
}

func (a *App) handleBriefSet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in agent.Brief
	if json.NewDecoder(io.LimitReader(r.Body, 1<<18)).Decode(&in) != nil {
		http.Error(w, "bad brief", http.StatusBadRequest)
		return
	}
	switch in.OnNewEntry {
	case "off", "others", "all":
	default:
		in.OnNewEntry = "off"
	}
	if in.Every != "" {
		if d, err := time.ParseDuration(in.Every); err != nil || d < 10*time.Minute {
			http.Error(w, "schedule must be a duration of at least 10m, e.g. 6h", http.StatusBadRequest)
			return
		}
	}
	rhythms := []agent.Rhythm{}
	for _, rh := range in.Rhythms {
		rh.Name, rh.At = strings.TrimSpace(rh.Name), strings.TrimSpace(rh.At)
		rh.Prompt, rh.Zone = strings.TrimSpace(rh.Prompt), strings.TrimSpace(rh.Zone)
		if rh.At == "" {
			continue
		}
		if ok, _ := rh.Due(time.Now(), "never"); !ok {
			// Due says no for a bad clock as well as for "not yet", so
			// check the format explicitly rather than by its answer.
			if _, _, good := agent.ParseClock(rh.At); !good {
				http.Error(w, "a time should look like 07:30", http.StatusBadRequest)
				return
			}
		}
		if rh.Name == "" {
			rh.Name = rh.At
		}
		if len(rhythms) >= 6 {
			break
		}
		rhythms = append(rhythms, rh)
	}
	ctx := r.Context()
	tl, err := a.Store.Thread(ctx, id)
	if err != nil {
		http.Error(w, "no such thread", http.StatusNotFound)
		return
	}
	prev, has := agent.LoadBrief(tl, a.Self)
	var refs *protolog.Refs
	if has {
		refs = &protolog.Refs{Supersedes: prev.ID}
	}
	body := map[string]any{"text": strings.TrimSpace(in.Text), "on_new_entry": in.OnNewEntry, "every": in.Every,
		"web": in.Web, "allow_domains": in.AllowDomains, "tools": in.Tools, "rhythms": rhythms}
	e, err := a.personAppend(ctx, id, protolog.Draft{Kind: agent.KindBrief, Lane: protolog.LaneContent, Refs: refs, Body: body})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if a.Scheduler != nil {
		a.Scheduler.Consume(ctx, id)
	}
	writeJSON(w, map[string]any{"id": e.ID})
}

func (a *App) handleRunNow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if msg, ok := a.agentReady(); !ok {
		writeJSON(w, map[string]any{"error": msg})
		return
	}
	ctx, cancel := runCtx()
	defer cancel()
	res := a.Runner.Run(ctx, agent.Trigger{Kind: agent.TriggerManual, Thread: id})
	if a.Scheduler != nil {
		a.Scheduler.Consume(ctx, id)
	}
	writeJSON(w, res)
}

// handleDecision records the person's answer and lets the agent continue.
func (a *App) handleDecision(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     string `json:"id"`
		Choice string `json:"choice"`
		Text   string `json:"text"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in) != nil || in.ID == "" || (in.Choice == "" && strings.TrimSpace(in.Text) == "") {
		http.Error(w, "id and a choice or text are required", http.StatusBadRequest)
		return
	}
	if msg, ok := a.agentReady(); !ok {
		writeJSON(w, map[string]any{"error": msg})
		return
	}
	ctx, cancel := runCtx()
	defer cancel()
	thread, decision := a.findEntry(ctx, in.ID)
	if decision == nil || decision.Kind != agent.KindDecision {
		http.Error(w, "no such decision", http.StatusNotFound)
		return
	}
	var db struct {
		Chain int `json:"chain"`
	}
	json.Unmarshal(decision.Body, &db)
	reply, err := a.personAppend(ctx, thread, protolog.Draft{Kind: agent.KindDecisionReply, Lane: protolog.LaneContent,
		Refs: &protolog.Refs{RepliesTo: in.ID},
		Body: map[string]any{"choice": in.Choice, "text": strings.TrimSpace(in.Text)}})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	res := a.Runner.Run(ctx, agent.Trigger{Kind: agent.TriggerDecision, Thread: thread, Entry: reply.ID, Chain: db.Chain})
	if a.Scheduler != nil {
		a.Scheduler.Consume(ctx, thread)
	}
	writeJSON(w, map[string]any{"reply": reply.ID, "answer": res.Answer, "outcome": res.Outcome, "error": res.Error})
}

func (a *App) findEntry(ctx context.Context, id string) (string, *protolog.Entry) {
	ids, err := a.Store.Threads(ctx)
	if err != nil {
		return "", nil
	}
	for _, t := range ids {
		if tl, err := a.Store.Thread(ctx, t); err == nil {
			if e := tl.Get(id); e != nil {
				return t, e
			}
		}
	}
	return "", nil
}

func (a *App) handleAgent(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"principal": a.AgentSelf, "name": a.displayName(a.AgentSelf),
		"person": a.Self, "model": a.Model, "reach": a.reach(), "revoked": a.agentRevoked}
	if msg, ok := a.agentReady(); !ok {
		out["problem"] = msg
	}
	if a.Scheduler != nil {
		out["status"] = a.Scheduler.Status(a.now())
	}
	n := 0
	if ids, err := a.Store.Threads(r.Context()); err == nil {
		for _, id := range ids {
			if tl, err := a.Store.Thread(r.Context(), id); err == nil {
				if perm.Fold(id, tl.Entries()).ActsFor(a.AgentSelf, a.Self) && a.AgentSelf != a.Self {
					n++
				}
			}
		}
	}
	out["delegated_threads"] = n
	writeJSON(w, out)
}

func (a *App) handleAgentRevoke(w http.ResponseWriter, r *http.Request) {
	if a.AgentSelf == "" {
		http.Error(w, "no agent", http.StatusBadRequest)
		return
	}
	n, err := agent.RevokeAgent(r.Context(), a.Store, a.DataDir, a.Key, a.Self, a.AgentSelf)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error(), "revoked_in": n})
		return
	}
	a.agentRevoked = true
	writeJSON(w, map[string]any{"ok": true, "revoked_in": n})
}

func (a *App) handleAgentConfig(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AllowDomains  []string `json:"allow_domains"`
		Brief         *string  `json:"brief"`
		AutoWeb       *string  `json:"auto_web"`
		Model         *string  `json:"model"`
		OpenRouterKey *string  `json:"openrouter_key"`
		ModelURL      *string  `json:"model_url"`
		ModelURLKey   *string  `json:"model_url_key"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in) != nil {
		http.Error(w, "bad config", http.StatusBadRequest)
		return
	}
	cfg, _ := agent.LoadConfig(a.DataDir)
	if in.AllowDomains != nil {
		clean := []string{}
		for _, d := range in.AllowDomains {
			d = strings.ToLower(strings.TrimSpace(d))
			if d != "" {
				clean = append(clean, d)
			}
		}
		cfg.AllowDomains = clean
	}
	if in.Brief != nil {
		cfg.Brief = strings.TrimSpace(*in.Brief)
	}
	if in.AutoWeb != nil {
		switch v := strings.TrimSpace(*in.AutoWeb); v {
		case "listed", "any", "off":
			cfg.AutoWeb = v
		default:
			http.Error(w, "that is not one of the choices", http.StatusBadRequest)
			return
		}
	}
	if in.Model != nil {
		want := strings.TrimSpace(*in.Model)
		if want != "" && a.Runner != nil && !a.Runner.MayChoose(cfg, want) {
			writeJSON(w, map[string]any{"error": "This node pays for the thinking, so the choice of model is limited. " +
				"Add an OpenRouter key of your own above and every model is yours."})
			return
		}
		cfg.Model = want
	}
	if in.OpenRouterKey != nil && strings.TrimSpace(*in.OpenRouterKey) != "" {
		if !a.mayHoldSecrets() {
			writeJSON(w, map[string]any{"error": "Add your email first. This node will not keep a key for an account nobody can recover."})
			return
		}
		cfg.OpenRouterKey = strings.TrimSpace(*in.OpenRouterKey)
	}
	if in.ModelURL != nil {
		u := strings.TrimSpace(*in.ModelURL)
		if u != "" && !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			http.Error(w, "model_url must start with http:// or https://", http.StatusBadRequest)
			return
		}
		if u != cfg.ModelURL {
			// A new endpoint does not inherit the last one's credential.
			cfg.ModelURLKey = ""
		}
		cfg.ModelURL = u
	}
	if in.ModelURLKey != nil && strings.TrimSpace(*in.ModelURLKey) != "" {
		if !a.mayHoldSecrets() {
			writeJSON(w, map[string]any{"error": "Add your email first before storing a credential."})
			return
		}
		cfg.ModelURLKey = strings.TrimSpace(*in.ModelURLKey)
	}
	if err := agent.SaveConfig(a.DataDir, cfg); err != nil {
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "reach": a.reach()})
}

// handleDeleteThread removes a thread the person stewards from this node.
//
// Ownership is stewardship: the creator is always a steward, and only
// stewards can delete here. Deletion is local. A copy a peer already pulled,
// or a page someone already read through a link, is theirs; what this does
// is stop the thread existing on this node and kill every live link to it.
func (a *App) handleDeleteThread(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()
	tl, err := a.Store.Thread(ctx, id)
	if err != nil {
		http.Error(w, "no such thread", http.StatusNotFound)
		return
	}
	st := perm.Fold(id, tl.Entries())
	if !st.Stewards[a.Self] {
		writeJSON(w, map[string]any{"error": "Only a steward of this thread can delete it. You can revoke your own access instead, or ask the owner."})
		return
	}
	shares := a.loadShares()
	killed := 0
	for i := range shares {
		if shares[i].Thread == id && !shares[i].Revoked {
			shares[i].Revoked = true
			killed++
		}
	}
	if killed > 0 {
		a.saveShares(shares)
	}
	if err := a.Store.DeleteThread(ctx, id); err != nil {
		http.Error(w, "could not delete", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "links_stopped": killed})
}

// The model list: what OpenRouter serves that can call tools, with prices,
// so a person picks by name and cost rather than by guessing an id. Cached
// for an hour; the list is public and changes slowly.
var modelCache struct {
	sync.Mutex
	at   time.Time
	body []byte
}

var modelVendors = []string{"openai/", "anthropic/", "google/", "moonshotai/", "deepseek/", "x-ai/", "qwen/", "meta-llama/", "mistralai/", "z-ai/", "minimax/"}

func (a *App) handleModels(w http.ResponseWriter, r *http.Request) {
	// A node that pays for the thinking offers a short menu, and says why.
	if a.Runner != nil {
		cfg, _ := agent.LoadConfig(a.DataDir)
		if choices := a.Runner.Choices(cfg); len(choices) > 0 {
			writeJSON(w, map[string]any{"models": pricedSubset(choices), "default": choices[0],
				"limited": true,
				"note":    "This node pays for the thinking, so the choice is limited. Add your own OpenRouter key and every model is yours."})
			return
		}
	}
	modelCache.Lock()
	defer modelCache.Unlock()
	if time.Since(modelCache.at) < time.Hour && modelCache.body != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelCache.body)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1/models", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, map[string]any{"error": "could not reach openrouter.ai: " + err.Error(), "models": []any{}})
		return
	}
	defer resp.Body.Close()
	var raw struct {
		Data []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Context int    `json:"context_length"`
			Pricing struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
			Supported []string `json:"supported_parameters"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&raw); err != nil {
		writeJSON(w, map[string]any{"error": "bad model list", "models": []any{}})
		return
	}
	type out struct {
		ID      string  `json:"id"`
		Name    string  `json:"name"`
		In      float64 `json:"in_per_m"`
		Out     float64 `json:"out_per_m"`
		Context int     `json:"context"`
	}
	var models []out
	for _, m := range raw.Data {
		tools := false
		for _, p := range m.Supported {
			if p == "tools" {
				tools = true
			}
		}
		if !tools {
			continue
		}
		vendor := false
		for _, v := range modelVendors {
			if strings.HasPrefix(m.ID, v) {
				vendor = true
			}
		}
		if !vendor || strings.Contains(m.ID, ":") {
			continue
		}
		var pin, pout float64
		fmt.Sscanf(m.Pricing.Prompt, "%g", &pin)
		fmt.Sscanf(m.Pricing.Completion, "%g", &pout)
		models = append(models, out{ID: m.ID, Name: m.Name, In: pin * 1e6, Out: pout * 1e6, Context: m.Context})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	body, _ := json.Marshal(map[string]any{"models": models, "default": agent.DefaultModel})
	modelCache.at, modelCache.body = time.Now(), body
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// pricedSubset describes a short menu without calling out to the provider,
// so a restricted node answers instantly and works offline.
func pricedSubset(ids []string) []map[string]any {
	known := map[string][2]float64{
		"openai/gpt-5.6-luna":        {0.20, 1.20},
		"openai/gpt-5.6-mini":        {0.25, 2.00},
		"anthropic/claude-haiku-4.5": {1.00, 5.00},
		"google/gemini-2.5-flash":    {0.30, 2.50},
	}
	out := []map[string]any{}
	for _, id := range ids {
		p := known[id]
		out = append(out, map[string]any{"id": id, "name": id, "in_per_m": p[0], "out_per_m": p[1], "context": 0})
	}
	return out
}
