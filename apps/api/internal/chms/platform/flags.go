package platform

import "sync"

type FlagSnapshot struct {
	Version int64
	Values  map[string]bool
}

type FeatureFlags struct {
	mu       sync.RWMutex
	snapshot FlagSnapshot
}

func NewFeatureFlags(version int64, values map[string]bool) *FeatureFlags {
	return &FeatureFlags{snapshot: FlagSnapshot{Version: version, Values: cloneFlags(values)}}
}
func (f *FeatureFlags) Enabled(name string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.snapshot.Values[name]
}
func (f *FeatureFlags) Snapshot() FlagSnapshot {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return FlagSnapshot{Version: f.snapshot.Version, Values: cloneFlags(f.snapshot.Values)}
}
func (f *FeatureFlags) Replace(version int64, values map[string]bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if version <= f.snapshot.Version {
		return false
	}
	f.snapshot = FlagSnapshot{Version: version, Values: cloneFlags(values)}
	return true
}
func cloneFlags(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
