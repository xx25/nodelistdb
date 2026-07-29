package storage

import (
	"context"
	"testing"
	"time"

	"github.com/nodelistdb/internal/cache"
)

func newPolicyTestStorage(t *testing.T) *CachedStorage {
	t.Helper()
	return &CachedStorage{
		cache:  cache.NewMemoryCache(&cache.MemoryConfig{}),
		keyGen: cache.NewKeyGenerator("ndb"),
		config: &CacheStorageConfig{Enabled: true, MaxSearchResults: 500},
	}
}

type sample struct {
	Value string `json:"value"`
}

// TestCachedFetchPtrDoesNotStoreNil is the policy the ten pointer-returning
// readers depend on. nil from them means "nothing here yet" - no test cycle
// inside the window, no row in flag_statistics - not "nothing here". Storing it
// turns a state a later import ends into a cached fact for the whole TTL, which
// for the historical readers is a day.
func TestCachedFetchPtrDoesNotStoreNil(t *testing.T) {
	cs := newPolicyTestStorage(t)
	key := cs.analyticsKey("policy:ptr")

	calls := 0
	fetchNil := func() (*sample, error) { calls++; return nil, nil }

	for i := 0; i < 3; i++ {
		got, err := cachedFetchPtr(cs, key, time.Hour, fetchNil)
		if err != nil || got != nil {
			t.Fatalf("call %d returned (%v, %v), want (nil, nil)", i, got, err)
		}
	}
	if calls != 3 {
		t.Errorf("storage was called %d times, want 3: a nil result must not be cached", calls)
	}

	// Once there is something to cache, it is cached.
	found := 0
	got, err := cachedFetchPtr(cs, key, time.Hour, func() (*sample, error) {
		found++
		return &sample{Value: "here"}, nil
	})
	if err != nil || got == nil || got.Value != "here" {
		t.Fatalf("got (%v, %v), want the fetched value", got, err)
	}
	got, err = cachedFetchPtr(cs, key, time.Hour, func() (*sample, error) {
		found++
		return nil, nil
	})
	if err != nil || got == nil || got.Value != "here" {
		t.Fatalf("second call returned (%v, %v), want the cached value", got, err)
	}
	if found != 1 {
		t.Errorf("storage was called %d times, want 1: a non-nil result must be cached", found)
	}
}

// TestCachedFetchPtrTreatsStoredNullAsMiss covers an entry written before the
// nil policy existed: "null" unmarshals without error, so without the check it
// would be served as a hit.
func TestCachedFetchPtrTreatsStoredNullAsMiss(t *testing.T) {
	cs := newPolicyTestStorage(t)
	key := cs.analyticsKey("policy:null")
	if err := cs.cache.Set(context.Background(), key, []byte("null"), time.Hour); err != nil {
		t.Fatal(err)
	}

	calls := 0
	got, err := cachedFetchPtr(cs, key, time.Hour, func() (*sample, error) {
		calls++
		return &sample{Value: "fresh"}, nil
	})
	if err != nil || got == nil || got.Value != "fresh" {
		t.Fatalf("got (%v, %v), want the freshly fetched value", got, err)
	}
	if calls != 1 {
		t.Error("a stored null was served as a cache hit")
	}
}

// TestCachedFetchSliceDoesNotStoreEmpty is the same policy for the analytics
// readers, which come back empty either because nothing matches or because the
// window excluded everything.
func TestCachedFetchSliceDoesNotStoreEmpty(t *testing.T) {
	cs := newPolicyTestStorage(t)
	key := cs.analyticsKey("policy:slice")

	calls := 0
	for i := 0; i < 3; i++ {
		got, err := cachedFetchSlice(cs, key, time.Hour, func() ([]sample, error) {
			calls++
			return nil, nil
		})
		if err != nil || len(got) != 0 {
			t.Fatalf("call %d returned (%v, %v), want an empty result", i, got, err)
		}
	}
	if calls != 3 {
		t.Errorf("storage was called %d times, want 3: an empty result must not be cached", calls)
	}
}

// TestCachedFetchStoresEveryAnswer pins the other half of the split: the
// single-value readers cache what they get, zero values included, because for
// them there is no not-found-yet state to confuse with a value.
func TestCachedFetchStoresEveryAnswer(t *testing.T) {
	cs := newPolicyTestStorage(t)
	key := cs.analyticsKey("policy:value")

	calls := 0
	fetch := func() (time.Time, error) { calls++; return time.Time{}, nil }
	for i := 0; i < 3; i++ {
		if _, err := cachedFetch(cs, key, time.Hour, fetch); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Errorf("storage was called %d times, want 1: a zero value is still an answer", calls)
	}
}

// TestWhoisDomainKeyKeepsTheAllNetworksSentinel pins the "*" spelling. An empty
// trailing segment would work, but it would orphan every entry written under
// the old key in the on-disk cache, which survives a redeploy.
func TestWhoisDomainKeyKeepsTheAllNetworksSentinel(t *testing.T) {
	cs := newPolicyTestStorage(t)
	if got, want := cs.analyticsKey("whois:results:v4", whoisDomainKey("")), "ndb:analytics:whois:results:v4:*"; got != want {
		t.Errorf("all-networks key = %q, want %q", got, want)
	}
	if got, want := cs.analyticsKey("whois:results:v4", whoisDomainKey("fsxnet")), "ndb:analytics:whois:results:v4:fsxnet"; got != want {
		t.Errorf("scoped key = %q, want %q", got, want)
	}
}
