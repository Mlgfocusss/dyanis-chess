package forum

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"dyanis-chess/backend/internal/store"
)

// Reports: a signed-in person (a guest too) complains of a private message, a forum message or topic
// or a player; the moderators (FORUM_ADMINS) see the queue and close each report, with or without
// acting on the forum. The text complained about is copied into the report when it is made, so the
// author deleting it does not erase the evidence.

// Limits and sizes.
const (
	MaxReportNoteRunes = 300
	ReportsPerHour     = 10
	reportListLimit    = 100
	snapshotRunes      = 5000
)

var reportKinds = map[string]bool{store.ReportMessage: true, store.ReportForumPost: true, store.ReportForumTopic: true, store.ReportUser: true}
var reportReasons = map[string]bool{"spam": true, "abuse": true, "cheating": true, "other": true}

// What a moderator may do when closing a report.
const (
	actionNone   = "none"
	actionDelete = "delete" // delete the reported forum message (or topic)
	actionLock   = "lock"   // close the topic of the reported forum message
)

func (s *Service) report(w http.ResponseWriter, r *http.Request) {
	u := s.writer(w, r)
	if u == nil {
		return
	}
	var req struct {
		Kind     string `json:"kind"`
		TargetID string `json:"targetId"`
		Reason   string `json:"reason"`
		Note     string `json:"note"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !reportKinds[req.Kind] {
		writeError(w, http.StatusBadRequest, "неизвестный вид жалобы")
		return
	}
	if !reportReasons[req.Reason] {
		writeError(w, http.StatusBadRequest, "выберите причину жалобы")
		return
	}
	if req.TargetID == "" || len(req.TargetID) > 32 {
		writeError(w, http.StatusNotFound, "на что жалоба, не найдено")
		return
	}
	note, _ := cleanBody(req.Note, MaxReportNoteRunes)
	rep := &store.ComplaintRow{ReporterID: u.ID, ReporterName: u.Name, Kind: req.Kind, TargetID: req.TargetID, Reason: req.Reason, Note: note, Created: s.cfg.Now()}
	if !s.fillTarget(w, r, u, rep) {
		return
	}
	if !s.allow("report:"+u.ID, ReportsPerHour, time.Hour) {
		writeError(w, http.StatusTooManyRequests, "слишком много жалоб за час, подождите")
		return
	}
	id, err := s.cfg.Store.CreateReport(r.Context(), rep)
	switch {
	case errors.Is(err, store.ErrReportExists):
		writeError(w, http.StatusConflict, "вы уже пожаловались на это: модератор рассмотрит жалобу")
	case err != nil:
		serverError(w, "create report", err)
	default:
		writeJSON(w, http.StatusCreated, map[string]uint64{"id": id})
	}
}

func notFoundReport(w http.ResponseWriter) bool {
	writeError(w, http.StatusNotFound, "на что жалоба, не найдено")
	return false
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// fillTarget checks that the thing complained about exists and may be reported by this person and copies
// its text and author into the report; false when it wrote the answer.
func (s *Service) fillTarget(w http.ResponseWriter, r *http.Request, u *store.User, rep *store.ComplaintRow) bool {
	ctx := r.Context()
	self := func() bool {
		writeError(w, http.StatusBadRequest, "на себя жаловаться нельзя")
		return false
	}
	switch rep.Kind {
	case store.ReportMessage:
		id, err := strconv.ParseUint(rep.TargetID, 10, 64)
		if err != nil {
			return notFoundReport(w)
		}
		m, err := s.cfg.Store.MessageByID(ctx, id)
		if errors.Is(err, store.ErrNotFound) || (err == nil && m.ToID != u.ID) { // only what was written to you
			return notFoundReport(w)
		}
		if err != nil {
			serverError(w, "report message", err)
			return false
		}
		rep.Snapshot, rep.TargetUserID, rep.TargetName = clip(m.Body, snapshotRunes), m.FromID, s.nameOf(r, m.FromID)
	case store.ReportForumPost:
		id, err := strconv.ParseUint(rep.TargetID, 10, 64)
		if err != nil {
			return notFoundReport(w)
		}
		p, err := s.cfg.Store.ForumPostByID(ctx, id)
		if errors.Is(err, store.ErrNotFound) || (err == nil && p.Deleted) {
			return notFoundReport(w)
		}
		if err != nil {
			serverError(w, "report post", err)
			return false
		}
		if p.AuthorID == u.ID {
			return self()
		}
		rep.Snapshot, rep.TargetUserID, rep.TargetName, rep.TopicID = clip(p.Body, snapshotRunes), p.AuthorID, p.AuthorName, p.TopicID
	case store.ReportForumTopic:
		id, err := strconv.ParseUint(rep.TargetID, 10, 64)
		if err != nil {
			return notFoundReport(w)
		}
		t, err := s.cfg.Store.ForumTopicByID(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			return notFoundReport(w)
		}
		if err != nil {
			serverError(w, "report topic", err)
			return false
		}
		if t.AuthorID == u.ID {
			return self()
		}
		snap := t.Title
		if posts, err := s.cfg.Store.ForumPosts(ctx, t.ID, 1, 0); err == nil && len(posts) > 0 && !posts[0].Deleted {
			snap += "\n\n" + posts[0].Body
		}
		rep.Snapshot, rep.TargetUserID, rep.TargetName, rep.TopicID = clip(snap, snapshotRunes), t.AuthorID, t.AuthorName, t.ID
	case store.ReportUser:
		if rep.TargetID == u.ID {
			return self()
		}
		p, err := s.cfg.Store.PublicUser(ctx, rep.TargetID)
		if errors.Is(err, store.ErrNotFound) {
			return notFoundReport(w)
		}
		if err != nil {
			serverError(w, "report user", err)
			return false
		}
		rep.Snapshot, rep.TargetUserID, rep.TargetName = p.Name, p.ID, p.Name
	}
	return true
}

func (s *Service) nameOf(r *http.Request, id string) string {
	if p, err := s.cfg.Store.PublicUser(r.Context(), id); err == nil {
		return p.Name
	}
	return ""
}

// ReportJSON is a report in the moderators' queue.
type ReportJSON struct {
	ID       uint64 `json:"id"`
	Kind     string `json:"kind"`
	Reason   string `json:"reason"`
	Note     string `json:"note,omitempty"`
	Snapshot string `json:"snapshot"`
	Reporter struct {
		ID   string `json:"id,omitempty"`
		Name string `json:"name"`
	} `json:"reporter"`
	Target struct {
		ID   string `json:"id,omitempty"`
		Name string `json:"name"`
	} `json:"target"`
	TargetID string `json:"targetId"`
	TopicID  uint64 `json:"topicId,omitempty"`
	Status   string `json:"status"`
	Action   string `json:"action,omitempty"`
	Created  int64  `json:"created"`
	ClosedAt int64  `json:"closedAt,omitempty"`
}

// moderator returns the signed-in moderator or writes the answer.
func (s *Service) moderator(w http.ResponseWriter, r *http.Request) *store.User {
	u := s.writer(w, r)
	if u == nil {
		return nil
	}
	if !s.isAdmin(u) {
		writeError(w, http.StatusForbidden, "это может делать только модератор")
		return nil
	}
	return u
}

func (s *Service) reports(w http.ResponseWriter, r *http.Request) {
	if s.moderator(w, r) == nil {
		return
	}
	status := r.URL.Query().Get("status")
	switch status {
	case "":
		status = store.ReportOpen
	case store.ReportOpen, store.ReportClosed:
	default:
		writeError(w, http.StatusBadRequest, "статус: open или closed")
		return
	}
	rows, err := s.cfg.Store.Reports(r.Context(), status, reportListLimit)
	if err != nil {
		serverError(w, "reports", err)
		return
	}
	items := make([]ReportJSON, 0, len(rows))
	for _, x := range rows {
		j := ReportJSON{ID: x.ID, Kind: x.Kind, Reason: x.Reason, Note: x.Note, Snapshot: x.Snapshot, TargetID: x.TargetID, TopicID: x.TopicID,
			Status: x.Status, Action: x.Action, Created: x.Created.UnixMilli()}
		j.Reporter.ID, j.Reporter.Name = x.ReporterID, x.ReporterName
		j.Target.ID, j.Target.Name = x.TargetUserID, x.TargetName
		if !x.ClosedAt.IsZero() {
			j.ClosedAt = x.ClosedAt.UnixMilli()
		}
		items = append(items, j)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Service) closeReport(w http.ResponseWriter, r *http.Request) {
	mod := s.moderator(w, r)
	if mod == nil {
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "жалоба не найдена")
		return
	}
	var req struct {
		Action string `json:"action"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Action == "" {
		req.Action = actionNone
	}
	ctx := r.Context()
	rep, err := s.cfg.Store.Report(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "жалоба не найдена")
		return
	}
	if err != nil {
		serverError(w, "report", err)
		return
	}
	if rep.Status != store.ReportOpen {
		writeError(w, http.StatusConflict, "жалоба уже закрыта")
		return
	}
	forumKind := rep.Kind == store.ReportForumPost || rep.Kind == store.ReportForumTopic
	switch req.Action {
	case actionNone:
	case actionDelete:
		if !forumKind {
			writeError(w, http.StatusBadRequest, "удалить можно только сообщение или тему форума")
			return
		}
		target, err := strconv.ParseUint(rep.TargetID, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "неверная цель жалобы")
			return
		}
		if rep.Kind == store.ReportForumPost {
			err = s.cfg.Store.ForumDeletePost(ctx, target)
		} else {
			err = s.cfg.Store.ForumDeleteTopic(ctx, target)
		}
		if err != nil {
			serverError(w, "report delete", err)
			return
		}
	case actionLock:
		if !forumKind || rep.TopicID == 0 {
			writeError(w, http.StatusBadRequest, "закрыть можно только тему форума")
			return
		}
		if err := s.cfg.Store.ForumSetFlag(ctx, rep.TopicID, "locked", true); err != nil {
			serverError(w, "report lock", err)
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "действие: none, delete или lock")
		return
	}
	n, err := s.cfg.Store.CloseReports(ctx, rep.Kind, rep.TargetID, mod.ID, req.Action, s.cfg.Now())
	if err != nil {
		serverError(w, "close report", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"closed": n, "action": req.Action})
}
