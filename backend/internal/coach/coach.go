// Package coach is the AI coach of the review: it explains a move of a
// reviewed game in words. The chess itself is the engine's (the evaluations
// and the lines come from the review report); a language model, reached
// through OpenRouter, only puts those facts into a short explanation, and the
// prompt forbids it to name a move that is not in them. What leaves the
// server is the position, the move and the engine's lines: no names, no
// accounts.
package coach

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultModels are free OpenRouter models, best first; the coach goes down the list when one of them
// fails. Free models come and go and are often rate-limited upstream (measured 2026-10-20: only the
// first answered every time, the others answered now and then), so a list is needed. A paid model
// put last in OPENROUTER_MODELS (say anthropic/claude-haiku-5.5) would be the last resort.
var DefaultModels = []string{
	"nvidia/nemotron-3-ultra-550b-a55b:free",
	"google/gemma-4-31b-it:free",
	"google/gemma-4-26b-a4b-it:free",
	"poolside/laguna-s-2.1:free",
}

// cooldown is how long a model that failed is left alone before the next try.
const cooldown = 90 * time.Second

// attemptTimeout bounds one model's answer: the free ones sometimes hang.
const attemptTimeout = 25 * time.Second

// Errors the service gives; the caller turns them into a reply.
var (
	ErrDisabled = errors.New("coach: no OpenRouter key")
	ErrLimit    = errors.New("coach: hourly limit reached")
	ErrUpstream = errors.New("coach: the model did not answer")
)

// Store keeps explanations between runs, by the hash of the question.
type Store interface {
	LoadCoachNote(ctx context.Context, hash string) (text string, ok bool, err error)
	SaveCoachNote(ctx context.Context, hash, model, text string) error
}

// Config tunes a Service. Zero values get the defaults noted.
type Config struct {
	APIKey  string       // OpenRouter key; empty turns the coach off
	Models  []string     // OpenRouter model ids to try in turn; default DefaultModels
	BaseURL string       // default https://openrouter.ai/api/v1
	Site    string       // shown to OpenRouter as the referrer, e.g. the public URL
	HTTP    *http.Client // default: 45 s timeout
	Store   Store        // optional
	PerHour int          // new explanations one person may ask for in an hour; default 40
	Slots   int          // requests to the model at once; default 4
}

// Note is an explanation.
type Note struct {
	Text   string `json:"text"`
	Cached bool   `json:"cached,omitempty"`
}

// Service asks the model. Safe for concurrent use.
type Service struct {
	cfg   Config
	slots chan struct{}

	mu    sync.Mutex
	memo  map[string]string
	order []string // hashes in the order they were remembered
	asked map[string][]time.Time
	cool  map[string]time.Time // model -> when it may be tried again
}

const memoSize = 2000

// New returns a Service; with no key it is off (Enabled reports false).
func New(cfg Config) *Service {
	if len(cfg.Models) == 0 {
		cfg.Models = DefaultModels
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://openrouter.ai/api/v1"
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 45 * time.Second}
	}
	if cfg.PerHour <= 0 {
		cfg.PerHour = 40
	}
	if cfg.Slots <= 0 {
		cfg.Slots = 4
	}
	return &Service{
		cfg:   cfg,
		slots: make(chan struct{}, cfg.Slots),
		memo:  map[string]string{},
		asked: map[string][]time.Time{},
		cool:  map[string]time.Time{},
	}
}

// Enabled says whether the coach has a key to work with.
func (s *Service) Enabled() bool { return s != nil && s.cfg.APIKey != "" }

// Explain writes the explanation of a move. who names the asker (a user or an
// address) for the hourly limit; explanations already written are free.
func (s *Service) Explain(ctx context.Context, who string, f Facts) (Note, error) {
	if !s.Enabled() {
		return Note{}, ErrDisabled
	}
	prompt := Prompt(f)
	// The hash does not name a model: an answer kept from any of them serves the same question.
	sum := sha256.Sum256([]byte(systemPrompt + "\n" + prompt))
	hash := hex.EncodeToString(sum[:])

	if text, ok := s.remembered(hash); ok {
		return Note{Text: text, Cached: true}, nil
	}
	if s.cfg.Store != nil {
		text, ok, err := s.cfg.Store.LoadCoachNote(ctx, hash)
		if err != nil {
			log.Printf("coach: load note: %v", err)
		} else if ok {
			s.remember(hash, text)
			return Note{Text: text, Cached: true}, nil
		}
	}

	if !s.allow(who) {
		return Note{}, ErrLimit
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return Note{}, ctx.Err()
	}
	text, model, err := s.ask(ctx, prompt)
	if err != nil {
		return Note{}, err
	}
	s.remember(hash, text)
	if s.cfg.Store != nil {
		if err := s.cfg.Store.SaveCoachNote(ctx, hash, model, text); err != nil {
			log.Printf("coach: save note: %v", err)
		}
	}
	return Note{Text: text}, nil
}

func (s *Service) remembered(hash string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.memo[hash]
	return t, ok
}

func (s *Service) remember(hash, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.memo[hash]; !ok {
		s.order = append(s.order, hash)
		if len(s.order) > memoSize {
			delete(s.memo, s.order[0])
			s.order = s.order[1:]
		}
	}
	s.memo[hash] = text
}

// allow counts a new explanation against who's hour.
func (s *Service) allow(who string) bool {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	recent := s.asked[who][:0]
	for _, t := range s.asked[who] {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	if len(recent) >= s.cfg.PerHour {
		s.asked[who] = recent
		return false
	}
	s.asked[who] = append(recent, now)
	if len(s.asked) > 5000 { // forget people who have not asked for an hour
		for k, ts := range s.asked {
			if len(ts) == 0 || now.Sub(ts[len(ts)-1]) >= time.Hour {
				delete(s.asked, k)
			}
		}
	}
	return true
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
	// The models that think before they answer would spend the whole budget on it.
	Reasoning map[string]any `json:"reasoning"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// errBadKey stops the walk down the list: no other model will accept the key either.
var errBadKey = errors.New("the key was refused")

// ask puts the question to the models in turn and returns the first answer, with the model that
// gave it. A model that failed (rate-limited, gone, silent, empty) is left alone for a while, so
// the next questions do not wait for it again; if all are resting they are tried anyway.
func (s *Service) ask(ctx context.Context, prompt string) (text, model string, err error) {
	models := s.modelOrder()
	var lastErr error
	for _, m := range models {
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}
		began := time.Now()
		text, err := s.try(ctx, m, prompt)
		if err == nil {
			log.Printf("coach: %s answered in %d ms", m, time.Since(began).Milliseconds())
			return text, m, nil
		}
		lastErr = err
		log.Printf("coach: %s: %v", m, err)
		if errors.Is(err, errBadKey) {
			break
		}
		s.mu.Lock()
		s.cool[m] = time.Now().Add(cooldown)
		s.mu.Unlock()
	}
	log.Printf("coach: no model answered (last: %v)", lastErr)
	return "", "", ErrUpstream
}

// modelOrder lists the models to try: those not resting first, in the configured order.
func (s *Service) modelOrder() []string {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	var ready, resting []string
	for _, m := range s.cfg.Models {
		if now.Before(s.cool[m]) {
			resting = append(resting, m)
		} else {
			ready = append(ready, m)
		}
	}
	return append(ready, resting...)
}

// try asks one model.
func (s *Service) try(ctx context.Context, model, prompt string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:       model,
		Messages:    []chatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: prompt}},
		MaxTokens:   700,
		Temperature: 0.3,
		Reasoning:   map[string]any{"enabled": false},
	})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Title", "Dyanis Chess")
	if s.cfg.Site != "" {
		req.Header.Set("HTTP-Referer", s.cfg.Site)
	}
	resp, err := s.cfg.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read answer: %w", err)
	}
	var out chatResponse
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode != http.StatusOK || out.Error != nil {
		msg := ""
		if out.Error != nil {
			msg = out.Error.Message
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return "", fmt.Errorf("status 401 (%s): %w", msg, errBadKey)
		}
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, msg)
	}
	if len(out.Choices) == 0 {
		return "", errors.New("no choices in the answer")
	}
	text := Clean(out.Choices[0].Message.Content)
	if text == "" {
		return "", errors.New("empty answer")
	}
	return text, nil
}

// Clean removes what a plain explanation should not have: markdown marks and
// the blank space around it.
func Clean(s string) string {
	s = strings.NewReplacer("**", "", "__", "", "`", "").Replace(s)
	lines := strings.Split(strings.TrimSpace(s), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "#>*- "))
		if l == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
