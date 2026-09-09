package variably

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SDK-side prompt config store + background poll.
//
// Bootstraps the project's prompt-experiment config from GET /internal/sdk/prompts/config
// once, refreshes it in the background with ETag/If-None-Match (a 304 is a no-op), and
// assigns variants locally from the cached snapshot — so a per-event assignment costs no
// backend call. Mirrors the Python SDK's PromptConfigStore.

// DefaultPromptPollInterval matches the Python SDK, so every SDK revalidates on the
// same cadence.
const DefaultPromptPollInterval = 45 * time.Second

// PromptConfigVariant is one variant in the snapshot: enough to assign deterministically
// and to serve the prompt.
type PromptConfigVariant struct {
	VariantID      string                 `json:"variant_id"`
	Name           string                 `json:"name"`
	Weight         float64                `json:"weight"`
	IsControl      bool                   `json:"is_control"`
	PromptTemplate string                 `json:"prompt_template"`
	Parameters     map[string]interface{} `json:"parameters,omitempty"`
}

// PromptConfigExperiment is one active experiment in the snapshot.
type PromptConfigExperiment struct {
	ExperimentID      string                `json:"experiment_id"`
	ExperimentKey     string                `json:"experiment_key"`
	Status            string                `json:"status"`
	TrafficAllocation int                   `json:"traffic_allocation"`
	Variants          []PromptConfigVariant `json:"variants"`
}

// PromptConfig is the whole snapshot. Version doubles as the ETag validator.
type PromptConfig struct {
	Version     string                   `json:"version"`
	Experiments []PromptConfigExperiment `json:"experiments"`
}

// PromptConfigFetcher takes the current ETag and returns the new config on change, or
// nil when unchanged (304). Injectable so the store is testable without a live server.
type PromptConfigFetcher func(ctx context.Context, etag string) (*PromptConfig, string, error)

// PromptConfigStore holds a thread-safe snapshot and assigns variants from it.
type PromptConfigStore struct {
	fetcher PromptConfigFetcher

	mu          sync.RWMutex
	etag        string
	experiments map[string]PromptConfigExperiment // experiment_key -> experiment

	stopOnce sync.Once
	stop     chan struct{}
	wg       sync.WaitGroup
}

// NewPromptConfigStore builds a store. Nothing is fetched until Refresh or StartPolling.
func NewPromptConfigStore(fetcher PromptConfigFetcher) *PromptConfigStore {
	return &PromptConfigStore{
		fetcher:     fetcher,
		experiments: make(map[string]PromptConfigExperiment),
		stop:        make(chan struct{}),
	}
}

// Refresh fetches the config once, replacing the snapshot if it changed.
//
// A 304 or an error leaves the previous snapshot in place: a transient failure must not
// blank the config and silently send every user to control.
func (s *PromptConfigStore) Refresh(ctx context.Context) error {
	s.mu.RLock()
	etag := s.etag
	s.mu.RUnlock()

	cfg, newETag, err := s.fetcher(ctx, etag)
	if err != nil {
		return err
	}
	if cfg == nil {
		return nil // unchanged
	}

	next := make(map[string]PromptConfigExperiment, len(cfg.Experiments))
	for _, exp := range cfg.Experiments {
		next[exp.ExperimentKey] = exp
	}

	s.mu.Lock()
	s.experiments = next
	s.etag = newETag
	s.mu.Unlock()
	return nil
}

// StartPolling refreshes in the background until Close. Safe to call once; subsequent
// calls are ignored.
func (s *PromptConfigStore) StartPolling(interval time.Duration) {
	if interval <= 0 {
		interval = DefaultPromptPollInterval
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), interval)
				_ = s.Refresh(ctx) // a failed poll keeps the previous snapshot
				cancel()
			}
		}
	}()
}

// Assign returns the variant for a user, or nil when the experiment is not in the
// snapshot. A nil result means "ask the server" — the experiment may have been created
// between polls, and silently dropping it would hide a brand-new experiment.
func (s *PromptConfigStore) Assign(experimentKey, userID, sessionID string) *PromptConfigVariant {
	s.mu.RLock()
	exp, ok := s.experiments[experimentKey]
	s.mu.RUnlock()

	if !ok {
		return nil
	}

	key := KeyFromContext(map[string]interface{}{
		"user_id":    userID,
		"session_id": sessionID,
	})
	return SelectVariant(exp.Variants, exp.ExperimentID, key)
}

// Version reports the snapshot's version, or "" before the first successful fetch.
func (s *PromptConfigStore) Version() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.etag
}

// Close stops background polling and waits for the poller to exit.
func (s *PromptConfigStore) Close() {
	s.stopOnce.Do(func() { close(s.stop) })
	s.wg.Wait()
}

// HTTPPromptConfigFetcher fetches the config over HTTP with ETag revalidation.
func HTTPPromptConfigFetcher(baseURL, apiKey string, client *http.Client) PromptConfigFetcher {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	return func(ctx context.Context, etag string) (*PromptConfig, string, error) {
		req, err := http.NewRequestWithContext(
			ctx, http.MethodGet, baseURL+"/api/v1/internal/sdk/prompts/config", nil,
		)
		if err != nil {
			return nil, "", fmt.Errorf("build prompt config request: %w", err)
		}
		req.Header.Set("X-API-Key", apiKey)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("fetch prompt config: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		// Unchanged since the last poll; the caller keeps what it has.
		if resp.StatusCode == http.StatusNotModified {
			return nil, etag, nil
		}
		if resp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("prompt config responded %d", resp.StatusCode)
		}

		var cfg PromptConfig
		if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
			return nil, "", fmt.Errorf("decode prompt config: %w", err)
		}

		newETag := resp.Header.Get("ETag")
		if newETag == "" {
			newETag = cfg.Version
		}
		return &cfg, newETag, nil
	}
}
