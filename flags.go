package variably

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Feature-flag evaluation against the public SDK surface (/api/v1/sdk/*).
//
// Kept separate from dynamic configs: they are different backend features with
// different storage, and evaluating a flag as though it were a config silently returns
// the caller's default for every flag.

// FlagNotFoundReason is what the API reports when a project has no such flag.
const FlagNotFoundReason = "gate_not_found"

// FlagNotFoundError reports a flag that is not defined in the project.
//
// This is a configuration error, not a value. Returning the caller's default here
// would make a typo, an unseeded flag, or a half-finished migration look exactly like
// a flag that is deliberately off — the application would run on defaults and appear
// healthy. Network failures are different and still fall back to the default: those
// are transient, this one is not.
type FlagNotFoundError struct {
	FlagKey string
}

func (e *FlagNotFoundError) Error() string {
	return fmt.Sprintf(
		"variably: flag %q is not defined in this project. Create it, or check the key "+
			"for a typo — a missing flag is not treated as \"off\"", e.FlagKey,
	)
}

// IsFlagNotFound reports whether err is a FlagNotFoundError.
func IsFlagNotFound(err error) bool {
	var notFound *FlagNotFoundError
	return errors.As(err, &notFound)
}

const (
	sdkEvaluatePath      = "/api/v1/sdk/evaluate"
	sdkEvaluateBatchPath = "/api/v1/sdk/evaluate/batch"
	sdkEvaluateAllPath   = "/api/v1/sdk/evaluate/all"
	sdkEvaluateGatePath  = "/api/v1/sdk/feature-gates/evaluate"
)

// DefaultFlagSnapshotTTL matches the Python SDK and the JavaScript flag SDKs, so every
// SDK revalidates on the same cadence.
const DefaultFlagSnapshotTTL = 45 * time.Second

// sdkContext is the wire shape the server expects. UserContext's own json tags are
// snake_case and would not bind, so the mapping is explicit rather than implicit.
type sdkContext struct {
	UserID     string                 `json:"userId,omitempty"`
	Email      string                 `json:"email,omitempty"`
	Country    string                 `json:"country,omitempty"`
	Language   string                 `json:"language,omitempty"`
	Platform   string                 `json:"platform,omitempty"`
	Version    string                 `json:"version,omitempty"`
	IPAddress  string                 `json:"ipAddress,omitempty"`
	UserAgent  string                 `json:"userAgent,omitempty"`
	SessionID  string                 `json:"sessionId,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

func toSDKContext(uc UserContext) sdkContext {
	return sdkContext{
		UserID:     uc.UserID,
		Email:      uc.Email,
		Country:    uc.Country,
		Language:   uc.Language,
		Platform:   uc.Platform,
		Version:    uc.Version,
		IPAddress:  uc.IPAddress,
		UserAgent:  uc.UserAgent,
		SessionID:  uc.SessionID,
		Attributes: uc.Attributes,
	}
}

// contextKey builds a stable cache key. Only fields that can affect evaluation are
// included, so contexts differing in irrelevant metadata share a snapshot.
func contextKey(uc UserContext) string {
	var b strings.Builder
	b.WriteString(uc.UserID)
	b.WriteByte('|')
	b.WriteString(uc.Email)
	b.WriteByte('|')
	b.WriteString(uc.Country)
	b.WriteByte('|')
	b.WriteString(uc.SessionID)

	if len(uc.Attributes) > 0 {
		keys := make([]string, 0, len(uc.Attributes))
		for k := range uc.Attributes {
			keys = append(keys, k)
		}
		sort.Strings(keys) // map order is random; the key must not be
		for _, k := range keys {
			fmt.Fprintf(&b, "|%s=%v", k, uc.Attributes[k])
		}
	}
	return b.String()
}

type sdkFlagResponse struct {
	FlagKey   string      `json:"flag_key"`
	Value     interface{} `json:"value"`
	Reason    string      `json:"reason"`
	RuleID    string      `json:"rule_id,omitempty"`
	VariantID string      `json:"variant_id,omitempty"`
}

type sdkBatchResponse struct {
	Results map[string]sdkFlagResponse `json:"results"`
}

type sdkAllFlagsResponse struct {
	Flags map[string]sdkFlagResponse `json:"flags"`
	ETag  string                     `json:"etag,omitempty"`
}

func (r sdkFlagResponse) toFlagResult(key string, cacheHit bool) FlagResult {
	return FlagResult{
		Key:         key,
		Value:       r.Value,
		Reason:      r.Reason,
		RuleID:      r.RuleID,
		Variation:   r.VariantID,
		EvaluatedAt: time.Now(),
		CacheHit:    cacheHit,
	}
}

// postSDK issues an authenticated request to the public SDK surface. A 304 is returned
// to the caller rather than treated as an error.
func (c *VariablyClient) postSDK(
	ctx context.Context, path string, body interface{}, etag string,
) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.BaseURL+path, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.config.APIKey)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", path, err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotModified {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%s responded %d", path, resp.StatusCode)
	}
	return resp, nil
}

// evaluateFlagRemote evaluates one flag over HTTP.
func (c *VariablyClient) evaluateFlagRemote(
	ctx context.Context, flagKey string, userContext UserContext,
) (FlagResult, error) {
	resp, err := c.postSDK(ctx, sdkEvaluatePath, map[string]interface{}{
		"flag_key": flagKey,
		"context":  toSDKContext(userContext),
	}, "")
	if err != nil {
		return FlagResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var body sdkFlagResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return FlagResult{}, fmt.Errorf("decode flag response: %w", err)
	}
	return body.toFlagResult(flagKey, false), nil
}

// evaluateFlagsRemote evaluates many flags in one request.
func (c *VariablyClient) evaluateFlagsRemote(
	ctx context.Context, flagKeys []string, userContext UserContext,
) (map[string]FlagResult, error) {
	resp, err := c.postSDK(ctx, sdkEvaluateBatchPath, map[string]interface{}{
		"flag_keys": flagKeys,
		"context":   toSDKContext(userContext),
	}, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var body sdkBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode batch response: %w", err)
	}

	results := make(map[string]FlagResult, len(body.Results))
	for key, r := range body.Results {
		results[key] = r.toFlagResult(key, false)
	}
	return results, nil
}

// ---------- all-flags snapshot ----------

type flagSnapshot struct {
	flags     map[string]FlagResult
	etag      string
	fetchedAt time.Time
}

// flagSnapshotStore holds evaluated flag sets per context and revalidates with ETag.
//
// Without it every evaluation is a network round trip, which on a request-handling path
// means one per request. A stale snapshot is served immediately and refreshed behind the
// caller, so revalidation never adds latency to a live request.
type flagSnapshotStore struct {
	mu        sync.RWMutex
	entries   map[string]*flagSnapshot
	inFlight  map[string]chan struct{}
	ttl       time.Duration
	maxSize   int
	enabled   bool
}

func newFlagSnapshotStore(cache CacheConfig) *flagSnapshotStore {
	ttl := cache.TTL
	if ttl <= 0 {
		ttl = DefaultFlagSnapshotTTL
	}
	maxSize := cache.MaxSize
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &flagSnapshotStore{
		entries:  make(map[string]*flagSnapshot),
		inFlight: make(map[string]chan struct{}),
		ttl:      ttl,
		maxSize:  maxSize,
		enabled:  cache.Enabled,
	}
}

func (s *flagSnapshotStore) get(key string) (*flagSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[key]
	return entry, ok
}

func (s *flagSnapshotStore) put(key string, entry *flagSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for len(s.entries) >= s.maxSize {
		var oldestKey string
		oldest := time.Now().Add(time.Hour)
		for k, e := range s.entries {
			if e.fetchedAt.Before(oldest) {
				oldest = e.fetchedAt
				oldestKey = k
			}
		}
		if oldestKey == "" {
			break
		}
		delete(s.entries, oldestKey)
	}
	s.entries[key] = entry
}

// AllFlags returns every flag in the project evaluated for one context.
//
// Backs start-up and server-side rendering, where the caller does not know which flag
// keys exist. The result is cached per context and revalidated with ETag, so repeated
// calls cost nothing on the wire until the snapshot ages out.
func (c *VariablyClient) AllFlags(ctx context.Context, userContext UserContext) (map[string]FlagResult, error) {
	key := contextKey(userContext)

	if !c.flagSnapshots.enabled {
		snapshot, err := c.fetchAllFlags(ctx, userContext, "")
		if err != nil {
			return nil, err
		}
		return snapshot.flags, nil
	}

	entry, ok := c.flagSnapshots.get(key)
	if !ok {
		snapshot, err := c.fetchAllFlags(ctx, userContext, "")
		if err != nil {
			return nil, err
		}
		c.flagSnapshots.put(key, snapshot)
		return snapshot.flags, nil
	}

	if time.Since(entry.fetchedAt) > c.flagSnapshots.ttl {
		// Refresh behind the caller. A failed refresh leaves the previous values in
		// place rather than flipping every flag to its default mid-request.
		go func() {
			bg, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if fresh, err := c.fetchAllFlags(bg, userContext, entry.etag); err == nil {
				c.flagSnapshots.put(key, fresh)
			}
		}()
	}

	return entry.flags, nil
}

// ValidateFlags reports which of expectedKeys the project does not define.
//
// Call this at start-up: it surfaces every missing flag at once, before a request path
// can hit whichever one happens to be read first.
func (c *VariablyClient) ValidateFlags(
	ctx context.Context, expectedKeys []string, userContext UserContext,
) ([]string, error) {
	defined, err := c.AllFlags(ctx, userContext)
	if err != nil {
		return nil, err
	}

	var missing []string
	for _, key := range expectedKeys {
		if _, ok := defined[key]; !ok {
			missing = append(missing, key)
		}
	}
	return missing, nil
}

func (c *VariablyClient) fetchAllFlags(
	ctx context.Context, userContext UserContext, etag string,
) (*flagSnapshot, error) {
	resp, err := c.postSDK(ctx, sdkEvaluateAllPath, map[string]interface{}{
		"context": toSDKContext(userContext),
	}, etag)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Unchanged: keep the values we already hold, restart the clock.
	if resp.StatusCode == http.StatusNotModified {
		if existing, ok := c.flagSnapshots.get(contextKey(userContext)); ok {
			return &flagSnapshot{flags: existing.flags, etag: etag, fetchedAt: time.Now()}, nil
		}
	}

	var body sdkAllFlagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode all-flags response: %w", err)
	}

	flags := make(map[string]FlagResult, len(body.Flags))
	for key, r := range body.Flags {
		flags[key] = r.toFlagResult(key, false)
	}

	newETag := resp.Header.Get("ETag")
	if newETag == "" {
		newETag = body.ETag
	}
	return &flagSnapshot{flags: flags, etag: newETag, fetchedAt: time.Now()}, nil
}
