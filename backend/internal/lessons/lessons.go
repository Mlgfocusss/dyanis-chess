package lessons

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"dyanis-chess/backend/internal/store"
)

// Store is the part of *store.Store the service uses.
type Store interface {
	LessonProgressOf(ctx context.Context, userID string) (map[string]store.LessonProgress, error)
	SaveLessonProgress(ctx context.Context, userID, lessonID string, step, mistakes int, done bool) (store.LessonProgress, error)
}

// Config tunes a Service.
type Config struct {
	Store Store
	// UserID names the signed-in user of a request, "" for nobody.
	UserID func(*http.Request) string
	// Library is the content; nil loads the embedded one (and panics when it is broken: the server
	// should not start with a bad lesson, see Load).
	Library *Library
}

// Service is the lessons API.
type Service struct {
	cfg Config
	lib *Library
}

// New returns a Service.
func New(cfg Config) *Service {
	if cfg.UserID == nil {
		cfg.UserID = func(*http.Request) string { return "" }
	}
	if cfg.Library == nil {
		lib, err := Load()
		if err != nil {
			panic(err)
		}
		cfg.Library = lib
	}
	return &Service{cfg: cfg, lib: cfg.Library}
}

// Handler returns the routes of the lessons API.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/lessons", s.catalog)
	mux.HandleFunc("GET /api/lessons/{id}", s.lesson)
	mux.HandleFunc("POST /api/lessons/{id}/progress", s.saveProgress)
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// ProgressJS is a person's progress in a lesson, as the browser sees it.
type ProgressJS struct {
	Step      int        `json:"step"`      // steps done
	Mistakes  int        `json:"mistakes"`  // wrong moves
	Completed bool       `json:"completed"` // finished at least once
	Done      *time.Time `json:"doneAt,omitempty"`
}

func progressJS(p store.LessonProgress) *ProgressJS {
	return &ProgressJS{Step: p.Step, Mistakes: p.Mistakes, Completed: p.Completed(), Done: p.CompletedAt}
}

// CatalogLesson is a lesson in the list.
type CatalogLesson struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Icon        string      `json:"icon"`
	Minutes     int         `json:"minutes"`
	Steps       int         `json:"steps"`
	Exercises   int         `json:"exercises"`
	Progress    *ProgressJS `json:"progress,omitempty"` // nil: not started (or nobody is signed in)
}

// CatalogSection is a section in the list.
type CatalogSection struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Lessons     []CatalogLesson `json:"lessons"`
}

// Catalog is the answer of GET /api/lessons.
type Catalog struct {
	Sections  []CatalogSection `json:"sections"`
	Total     int              `json:"total"`          // lessons in all
	Completed int              `json:"completed"`      // lessons the person has finished
	Next      string           `json:"next,omitempty"` // the lesson to go on with: the first unfinished one
}

func (s *Service) progressOf(w http.ResponseWriter, r *http.Request) (map[string]store.LessonProgress, bool) {
	userID := s.cfg.UserID(r)
	if userID == "" {
		return nil, true
	}
	m, err := s.cfg.Store.LessonProgressOf(r.Context(), userID)
	if err != nil {
		log.Printf("lessons: progress: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось загрузить прогресс")
		return nil, false
	}
	return m, true
}

func (s *Service) catalog(w http.ResponseWriter, r *http.Request) {
	prog, ok := s.progressOf(w, r)
	if !ok {
		return
	}
	out := Catalog{Total: s.lib.Total()}
	for _, sec := range s.lib.Sections {
		cs := CatalogSection{ID: sec.ID, Title: sec.Title, Description: sec.Description}
		for _, l := range sec.Lessons {
			b := s.lib.byID[l.ID]
			cl := CatalogLesson{ID: l.ID, Title: l.Title, Description: l.Description, Icon: l.Icon, Minutes: l.Minutes,
				Steps: len(l.Steps), Exercises: b.json.Exercises}
			if p, has := prog[l.ID]; has {
				cl.Progress = progressJS(p)
				if p.Completed() {
					out.Completed++
				}
			}
			if out.Next == "" && (cl.Progress == nil || !cl.Progress.Completed) {
				out.Next = l.ID
			}
			cs.Lessons = append(cs.Lessons, cl)
		}
		out.Sections = append(out.Sections, cs)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Service) lesson(w http.ResponseWriter, r *http.Request) {
	l := s.lib.Lesson(r.PathValue("id"))
	if l == nil {
		writeError(w, http.StatusNotFound, "такого урока нет")
		return
	}
	prog, ok := s.progressOf(w, r)
	if !ok {
		return
	}
	if p, has := prog[l.ID]; has {
		l.Progress = progressJS(p)
	}
	writeJSON(w, http.StatusOK, l)
}

type progressRequest struct {
	Step     int  `json:"step"`
	Mistakes int  `json:"mistakes"`
	Done     bool `json:"done"`
}

// maxMistakes keeps a nonsense count out of the table.
const maxMistakes = 999

func (s *Service) saveProgress(w http.ResponseWriter, r *http.Request) {
	userID := s.cfg.UserID(r)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "войдите, чтобы прогресс сохранился")
		return
	}
	id := r.PathValue("id")
	total := s.lib.StepCount(id)
	if total == 0 {
		writeError(w, http.StatusNotFound, "такого урока нет")
		return
	}
	var req progressRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	if req.Step < 0 || req.Step > total || req.Mistakes < 0 || req.Mistakes > maxMistakes {
		writeError(w, http.StatusBadRequest, "неверный шаг или число ошибок")
		return
	}
	// a lesson is finished by doing its last step, and only then
	if req.Done && req.Step != total {
		writeError(w, http.StatusBadRequest, "урок завершается на последнем шаге")
		return
	}
	done := req.Done || req.Step == total
	p, err := s.cfg.Store.SaveLessonProgress(r.Context(), userID, id, req.Step, req.Mistakes, done)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		log.Printf("lessons: save: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось сохранить прогресс")
		return
	}
	writeJSON(w, http.StatusOK, progressJS(p))
}
