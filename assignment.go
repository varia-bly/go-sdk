package variably

import "hash/fnv"

// Deterministic, sticky variant assignment.
//
// This is a port of the server's internal/assignment package. It must stay
// byte-for-byte equivalent to it: the server and the SDK both compute assignments, and
// any divergence would flip a user between variants depending on which side answered —
// corrupting the experiment rather than merely being inconsistent.
//
// It is a pure function of (variants, experimentID, key): no clock, no randomness, no
// I/O. FNV-1a is used because it is trivially reproducible in any language, so the Go,
// Python and JavaScript SDKs can all agree.
//
// assignment_test.go pins the shared golden vectors. If those fail, this file and the
// server have drifted — fix the drift rather than the expectations.

// Bucket maps (salt, key) to a stable value in [0, 100).
func Bucket(salt, key string) float64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(salt + ":" + key))
	return float64(h.Sum32()%10000) / 100.0
}

// KeyFromContext pulls a stable per-user bucketing key, preferring user_id then
// session_id. Empty when neither is present, in which case the caller falls back to
// control.
func KeyFromContext(ctx map[string]interface{}) string {
	for _, k := range []string{"user_id", "session_id"} {
		if v, ok := ctx[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// SelectVariant deterministically assigns one variant by weight, sticky on key.
//
// Weights are normalised to sum to 100; unset or zero weights fall back to an equal
// split. An empty key returns the control variant (or the first) deterministically
// rather than picking at random, so a caller with no user id still gets a stable answer.
func SelectVariant(variants []PromptConfigVariant, experimentID, key string) *PromptConfigVariant {
	if len(variants) == 0 {
		return nil
	}
	if key == "" {
		for i := range variants {
			if variants[i].IsControl {
				return &variants[i]
			}
		}
		return &variants[0]
	}

	total := 0.0
	for _, v := range variants {
		if v.Weight > 0 {
			total += v.Weight
		}
	}

	bucket := Bucket(experimentID, key) // [0, 100)
	cumulative := 0.0
	for i := range variants {
		w := variants[i].Weight
		if total > 0 {
			w = w / total * 100.0 // normalize
		} else {
			w = 100.0 / float64(len(variants)) // no weights set → equal split
		}
		cumulative += w
		if bucket < cumulative {
			return &variants[i]
		}
	}
	return &variants[len(variants)-1] // rounding safety
}
