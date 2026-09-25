package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
)

// Projects: channels that hold channels. See agent/project.go for why the
// membership lives only in the project.

func (a *App) newThread(ctx context.Context, title string) (string, error) {
	l, genesis, err := protolog.NewThreadWith(a.Key, title, false, nil)
	if err != nil {
		return "", err
	}
	if err := a.Store.AppendEntries(ctx, l.Entries()); err != nil {
		return "", err
	}
	return genesis.ID, nil
}

// POST /app/api/projects {title} makes a project.
func (a *App) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title string `json:"title"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&in) != nil || strings.TrimSpace(in.Title) == "" {
		http.Error(w, "a title is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	id, err := a.newThread(ctx, strings.TrimSpace(in.Title))
	if err == nil {
		_, err = a.personAppend(ctx, id, protolog.Draft{Kind: agent.KindProject, Lane: protolog.LaneContent,
			Body: map[string]any{"text": "This is a project. Its channels each have their own people; only you see them all here."}})
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"id": id, "title": strings.TrimSpace(in.Title)})
}

// POST /app/api/project/{id}/channels {title} makes a channel inside it.
func (a *App) handleCreateProjectChannel(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("id")
	var in struct {
		Title string `json:"title"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&in) != nil || strings.TrimSpace(in.Title) == "" {
		http.Error(w, "a title is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if !agent.ReadStructure(ctx, a.Store, a.Self).IsProject[pid] {
		http.Error(w, "no such project", http.StatusNotFound)
		return
	}
	id, err := a.newThread(ctx, strings.TrimSpace(in.Title))
	if err == nil {
		err = a.member(ctx, pid, id, "add")
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"id": id, "title": strings.TrimSpace(in.Title), "project": pid})
}

// POST /app/api/project/move {thread, project} puts a channel in a project,
// or takes it out when project is empty.
func (a *App) handleMoveToProject(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Thread  string `json:"thread"`
		Project string `json:"project"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&in) != nil || in.Thread == "" {
		http.Error(w, "thread is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	s := agent.ReadStructure(ctx, a.Store, a.Self)
	if s.IsProject[in.Thread] {
		writeJSON(w, map[string]any{"error": "A project cannot go inside another project."})
		return
	}
	if in.Project != "" && !s.IsProject[in.Project] {
		http.Error(w, "no such project", http.StatusNotFound)
		return
	}
	if old := s.Parent[in.Thread]; old != "" && old != in.Project {
		if err := a.member(ctx, old, in.Thread, "remove"); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	if in.Project != "" && s.Parent[in.Thread] != in.Project {
		if err := a.member(ctx, in.Project, in.Thread, "add"); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true})
}

// member records membership in the project, never in the channel: the
// channel's other participants must not learn it belongs to anything.
func (a *App) member(ctx context.Context, project, thread, op string) error {
	_, err := a.personAppend(ctx, project, protolog.Draft{Kind: agent.KindProjectMember, Lane: protolog.LaneContent,
		Body: map[string]any{"thread": thread, "op": op}})
	return err
}
