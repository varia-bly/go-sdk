package variably

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// Golden vectors for deterministic assignment.
//
// These pin the exact bucketing shared by the server (internal/assignment) and every
// SDK. They are not arbitrary expectations to be updated when they fail: a failure means
// this SDK and the server would assign a user to different variants, which corrupts a
// running experiment rather than merely disagreeing. Fix the drift, not the numbers.
func TestBucketGoldenVectors(t *testing.T) {
	cases := []struct {
		salt, key string
		want      float64
	}{
		{"exp-1", "user-1", bucketRef("exp-1", "user-1")},
		{"exp-1", "user-2", bucketRef("exp-1", "user-2")},
		{"exp-2", "user-1", bucketRef("exp-2", "user-1")},
		{"", "", bucketRef("", "")},
	}

	for _, tc := range cases {
		got := Bucket(tc.salt, tc.key)
		if got != tc.want {
			t.Errorf("Bucket(%q, %q) = %v, want %v", tc.salt, tc.key, got, tc.want)
		}
		if got < 0 || got >= 100 {
			t.Errorf("Bucket(%q, %q) = %v, outside [0, 100)", tc.salt, tc.key, got)
		}
	}
}

// bucketRef recomputes the reference value independently of Bucket's implementation, so
// the test would catch a change to the hash, the separator, or the modulus.
func bucketRef(salt, key string) float64 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	var h uint32 = offset32
	for _, b := range []byte(salt + ":" + key) {
		h ^= uint32(b)
		h *= prime32
	}
	return float64(h%10000) / 100.0
}

func TestSelectVariantIsSticky(t *testing.T) {
	variants := []PromptConfigVariant{
		{VariantID: "a", Name: "control", Weight: 50, IsControl: true},
		{VariantID: "b", Name: "treatment", Weight: 50},
	}

	first := SelectVariant(variants, "exp-1", "user-42")
	if first == nil {
		t.Fatal("expected a variant")
	}

	// The same user must land on the same variant every time; a user that flips
	// between variants pollutes the experiment's results.
	for i := 0; i < 100; i++ {
		again := SelectVariant(variants, "exp-1", "user-42")
		if again == nil || again.VariantID != first.VariantID {
			t.Fatalf("assignment not sticky: got %v, want %s", again, first.VariantID)
		}
	}
}

func TestSelectVariantEmptyKeyPicksControl(t *testing.T) {
	variants := []PromptConfigVariant{
		{VariantID: "a", Name: "treatment", Weight: 50},
		{VariantID: "b", Name: "control", Weight: 50, IsControl: true},
	}

	got := SelectVariant(variants, "exp-1", "")
	if got == nil || got.VariantID != "b" {
		t.Errorf("empty key = %v, want the control variant (b)", got)
	}
}

func TestSelectVariantRespectsWeights(t *testing.T) {
	// A 90/10 split should land the large majority in the heavy variant. Exact counts
	// are implementation-defined, so assert the direction rather than a magic number.
	variants := []PromptConfigVariant{
		{VariantID: "heavy", Weight: 90},
		{VariantID: "light", Weight: 10},
	}

	heavy := 0
	const n = 2000
	for i := 0; i < n; i++ {
		got := SelectVariant(variants, "exp-weights", fmt.Sprintf("user-%d", i))
		if got != nil && got.VariantID == "heavy" {
			heavy++
		}
	}

	if heavy < n*3/4 {
		t.Errorf("heavy variant got %d/%d, expected a clear majority", heavy, n)
	}
}

func TestSelectVariantNoVariants(t *testing.T) {
	if got := SelectVariant(nil, "exp-1", "user-1"); got != nil {
		t.Errorf("no variants = %v, want nil", got)
	}
}

func TestStoreKeepsSnapshotOnFetchFailure(t *testing.T) {
	calls := 0
	fetcher := func(ctx context.Context, etag string) (*PromptConfig, string, error) {
		calls++
		if calls == 1 {
			return &PromptConfig{
				Version: "v1",
				Experiments: []PromptConfigExperiment{{
					ExperimentID:  "exp-1",
					ExperimentKey: "greeting",
					Variants: []PromptConfigVariant{
						{VariantID: "a", IsControl: true, Weight: 100},
					},
				}},
			}, "v1", nil
		}
		return nil, "", errors.New("backend unavailable")
	}

	store := NewPromptConfigStore(fetcher)
	if err := store.Refresh(context.Background()); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if got := store.Assign("greeting", "user-1", ""); got == nil {
		t.Fatal("expected an assignment after the first refresh")
	}

	// The backend goes away. The snapshot must survive: blanking it would send every
	// user to a nil variant during an outage.
	if err := store.Refresh(context.Background()); err == nil {
		t.Fatal("expected the failing refresh to report an error")
	}
	if got := store.Assign("greeting", "user-1", ""); got == nil {
		t.Error("snapshot was dropped on a failed refresh")
	}
}

func TestStoreNotModifiedKeepsSnapshot(t *testing.T) {
	fetcher := func(ctx context.Context, etag string) (*PromptConfig, string, error) {
		if etag == "v1" {
			return nil, "v1", nil // 304
		}
		return &PromptConfig{
			Version: "v1",
			Experiments: []PromptConfigExperiment{{
				ExperimentID:  "exp-1",
				ExperimentKey: "greeting",
				Variants:      []PromptConfigVariant{{VariantID: "a", IsControl: true}},
			}},
		}, "v1", nil
	}

	store := NewPromptConfigStore(fetcher)
	_ = store.Refresh(context.Background())
	_ = store.Refresh(context.Background()) // 304 path

	if got := store.Assign("greeting", "user-1", ""); got == nil || got.VariantID != "a" {
		t.Errorf("after a 304 the snapshot changed: %v", got)
	}
	if store.Version() != "v1" {
		t.Errorf("version = %q, want v1", store.Version())
	}
}

func TestStoreUnknownExperimentReturnsNil(t *testing.T) {
	fetcher := func(ctx context.Context, etag string) (*PromptConfig, string, error) {
		return &PromptConfig{Version: "v1"}, "v1", nil
	}
	store := NewPromptConfigStore(fetcher)
	_ = store.Refresh(context.Background())

	// nil means "ask the server": an experiment created between polls must not be
	// silently treated as absent.
	if got := store.Assign("not-in-snapshot", "user-1", ""); got != nil {
		t.Errorf("unknown experiment = %v, want nil so the caller falls back", got)
	}
}

func TestStoreCloseStopsPolling(t *testing.T) {
	fetcher := func(ctx context.Context, etag string) (*PromptConfig, string, error) {
		return &PromptConfig{Version: "v1"}, "v1", nil
	}
	store := NewPromptConfigStore(fetcher)
	store.StartPolling(10 * time.Millisecond)
	time.Sleep(30 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		store.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not stop the poller")
	}
}
