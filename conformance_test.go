package variably

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Conformance runner for the Go SDK.
//
// Drives the SDK against sdks/conformance/spec.json, the same cases every other SDK
// runs. A disagreement between languages fails here rather than surfacing to a user as
// a flag that behaves differently depending on which service read it.
//
// Skipped unless VARIABLY_BASE_URL points at a backend seeded with
// sdks/conformance/fixtures.sql.

type conformanceSpec struct {
	Keys  map[string]string `json:"keys"`
	Cases []struct {
		Name    string            `json:"name"`
		Key     string            `json:"key"`
		Context map[string]string `json:"context"`
		Expect  map[string]struct {
			Value  interface{} `json:"value"`
			Reason string      `json:"reason"`
		} `json:"expect"`
	} `json:"cases"`
	UndefinedFlag struct {
		Key     string `json:"key"`
		FlagKey string `json:"flagKey"`
	} `json:"undefinedFlag"`
	ExpectedFlags struct {
		Key     string   `json:"key"`
		Present []string `json:"present"`
		Missing []string `json:"missing"`
	} `json:"expectedFlags"`
	Stickiness struct {
		Key         string `json:"key"`
		FlagKey     string `json:"flagKey"`
		UserID      string `json:"userId"`
		Repetitions int    `json:"repetitions"`
	} `json:"stickiness"`
}

func loadSpec(t *testing.T) conformanceSpec {
	t.Helper()
	path := filepath.Join("..", "conformance", "spec.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var spec conformanceSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	return spec
}

func conformanceClient(t *testing.T, baseURL, apiKey string) *VariablyClient {
	t.Helper()
	cfg := DefaultClientConfig()
	cfg.BaseURL, cfg.APIKey, cfg.ProjectID = baseURL, apiKey, "conformance"
	cfg.EnableRealtime = false
	// Caching would mask a per-case difference, and correctness is what is under test.
	cfg.Cache.Enabled = false

	client, err := NewVariablyClient(cfg, NewDefaultLogger(LogLevelError))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// equalValue compares against the spec's JSON-decoded expectations, where every number
// is a float64 and booleans and strings are themselves.
func equalValue(got, want interface{}) bool {
	if reflect.DeepEqual(got, want) {
		return true
	}
	gotNum, gotOK := got.(float64)
	wantNum, wantOK := want.(float64)
	return gotOK && wantOK && gotNum == wantNum
}

func TestConformance(t *testing.T) {
	baseURL := os.Getenv("VARIABLY_BASE_URL")
	if baseURL == "" {
		t.Skip("set VARIABLY_BASE_URL to a backend seeded with conformance fixtures")
	}

	spec := loadSpec(t)
	ctx := context.Background()

	for _, testCase := range spec.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			client := conformanceClient(t, baseURL, spec.Keys[testCase.Key])
			userContext := UserContext{UserID: testCase.Context["userId"]}

			// The same values must come back however they are asked for; a divergence
			// between single and snapshot is the class of bug this suite exists for.
			all, err := client.AllFlags(ctx, userContext)
			if err != nil {
				t.Fatalf("AllFlags: %v", err)
			}

			for flagKey, expected := range testCase.Expect {
				if got, ok := all[flagKey]; !ok {
					t.Errorf("%s missing from AllFlags", flagKey)
				} else if !equalValue(got.Value, expected.Value) {
					t.Errorf("AllFlags[%s] = %v (%T), want %v", flagKey, got.Value, got.Value, expected.Value)
				}

				single := client.EvaluateFlag(ctx, flagKey, nil, userContext)
				if single.Error != nil {
					t.Errorf("EvaluateFlag(%s): %v", flagKey, single.Error)
				} else if !equalValue(single.Value, expected.Value) {
					t.Errorf("EvaluateFlag(%s) = %v, want %v", flagKey, single.Value, expected.Value)
				}

				if expected.Reason != "" && single.Reason != expected.Reason {
					t.Errorf("EvaluateFlag(%s) reason = %q, want %q", flagKey, single.Reason, expected.Reason)
				}
			}
		})
	}

	t.Run("batch agrees with single and snapshot", func(t *testing.T) {
		// Batch is the path that silently returned false for every flag until the
		// scoping fix, so it is asserted against the same expectations as the others
		// rather than trusted.
		var expectations = map[string]interface{}{}
		var keys []string
		for _, testCase := range spec.Cases {
			if testCase.Key != "projectA" || testCase.Context["userId"] != "ordinary-user" {
				continue
			}
			for flagKey, expected := range testCase.Expect {
				expectations[flagKey] = expected.Value
				keys = append(keys, flagKey)
			}
		}
		if len(keys) == 0 {
			t.Skip("spec has no projectA/ordinary-user case to batch")
		}

		client := conformanceClient(t, baseURL, spec.Keys["projectA"])
		results := client.EvaluateFlags(ctx, keys, UserContext{UserID: "ordinary-user"})

		for _, key := range keys {
			result, ok := results[key]
			if !ok {
				t.Errorf("%s missing from EvaluateFlags", key)
				continue
			}
			if result.Error != nil {
				t.Errorf("EvaluateFlags[%s]: %v", key, result.Error)
				continue
			}
			if !equalValue(result.Value, expectations[key]) {
				t.Errorf("EvaluateFlags[%s] = %v, want %v", key, result.Value, expectations[key])
			}
		}
	})

	t.Run("undefined flag reports an error", func(t *testing.T) {
		u := spec.UndefinedFlag
		client := conformanceClient(t, baseURL, spec.Keys[u.Key])
		uc := UserContext{UserID: "some-user"}

		// A missing flag is a configuration error. Returning the caller's default
		// would make a typo or an unseeded flag indistinguishable from one that is
		// deliberately off.
		_, err := client.EvaluateFlagBool(ctx, u.FlagKey, true, uc)
		if err == nil {
			t.Fatal("expected an error for an undefined flag, got none")
		}
		if !IsFlagNotFound(err) {
			t.Errorf("error = %v, want a FlagNotFoundError", err)
		}
		if !strings.Contains(err.Error(), u.FlagKey) {
			t.Errorf("error %q does not name the offending flag %q", err, u.FlagKey)
		}

		all, allErr := client.AllFlags(ctx, uc)
		if allErr != nil {
			t.Fatalf("AllFlags: %v", allErr)
		}
		if _, present := all[u.FlagKey]; present {
			t.Errorf("%s present in the snapshot; an undefined flag should be absent", u.FlagKey)
		}
	})

	t.Run("ValidateFlags reports every missing key at once", func(t *testing.T) {
		e := spec.ExpectedFlags
		client := conformanceClient(t, baseURL, spec.Keys[e.Key])

		missing, err := client.ValidateFlags(ctx, append(append([]string{}, e.Present...), e.Missing...), UserContext{})
		if err != nil {
			t.Fatalf("ValidateFlags: %v", err)
		}
		sort.Strings(missing)
		want := append([]string{}, e.Missing...)
		sort.Strings(want)
		if !reflect.DeepEqual(missing, want) {
			t.Errorf("missing = %v, want %v", missing, want)
		}
	})

	t.Run("assignment is sticky", func(t *testing.T) {
		s := spec.Stickiness
		client := conformanceClient(t, baseURL, spec.Keys[s.Key])
		uc := UserContext{UserID: s.UserID}

		first := client.EvaluateFlag(ctx, s.FlagKey, nil, uc)
		for i := 1; i < s.Repetitions; i++ {
			again := client.EvaluateFlag(ctx, s.FlagKey, nil, uc)
			if !equalValue(again.Value, first.Value) {
				t.Fatalf("assignment flipped between evaluations: %v then %v", first.Value, again.Value)
			}
		}
	})
}
