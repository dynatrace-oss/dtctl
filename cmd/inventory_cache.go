package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dynatrace-oss/dtctl/pkg/config"
	"github.com/dynatrace-oss/dtctl/sdk/inventory"
)

// inventoryVerdictTTL is how long cached capability verdicts filter recipe
// listings. Capabilities are retention-scoped and change over days, so a
// day-old verdict is still a good filter — and only an `absent` verdict hides
// anything.
const inventoryVerdictTTL = 24 * time.Hour

// inventoryVerdicts are the capability verdicts of the last inventory run for
// one environment.
type inventoryVerdicts struct {
	GeneratedAt time.Time                    `json:"generatedAt"`
	Present     []string                     `json:"present,omitempty"`
	Absent      []inventory.CapabilityStatus `json:"absent,omitempty"`
	Unknown     []inventory.CapabilityStatus `json:"unknown,omitempty"`
	// Structural marks verdicts from the quick structural pass `get recipes`
	// runs on a cold cache: probe-shaped capabilities were not evaluated.
	Structural bool `json:"structural,omitempty"`
}

func (v *inventoryVerdicts) age() time.Duration { return time.Since(v.GeneratedAt) }

// absent reports whether a capability was found absent, with the evidence.
func (v *inventoryVerdicts) absent(name string) (string, bool) {
	for _, a := range v.Absent {
		if a.Name == name {
			return a.Evidence, true
		}
	}
	return "", false
}

// known reports whether the verdicts cover a capability at all.
func (v *inventoryVerdicts) known(name string) bool {
	for _, p := range v.Present {
		if p == name {
			return true
		}
	}
	_, ok := v.absent(name)
	return ok
}

func verdictsFromInventory(inv *inventory.Inventory, structural bool) *inventoryVerdicts {
	return &inventoryVerdicts{
		GeneratedAt: time.Now().UTC(),
		Present:     inv.Capabilities,
		Absent:      inv.Absent,
		Unknown:     inv.Unknown,
		Structural:  structural,
	}
}

// merge adds verdicts for capabilities v does not cover yet.
func (v *inventoryVerdicts) merge(other *inventoryVerdicts) {
	for _, p := range other.Present {
		if !v.known(p) {
			v.Present = append(v.Present, p)
		}
	}
	for _, a := range other.Absent {
		if !v.known(a.Name) {
			v.Absent = append(v.Absent, a)
		}
	}
}

// verdictCacheKey names an environment's verdicts: the context for the CLI,
// environment and principal for a session.
func verdictCacheKey(cfg *config.Config) (string, bool) {
	if runSession != nil {
		sum := sha256.Sum256([]byte(runSession.EnvironmentURL + "\x00" + runSession.Token))
		return hex.EncodeToString(sum[:16]), true
	}
	if cfg == nil || cfg.CurrentContext == "" {
		return "", false
	}
	return cfg.CurrentContext, true
}

var (
	sessionVerdictsMu sync.Mutex
	sessionVerdicts   = map[string]*inventoryVerdicts{}
)

// loadInventoryVerdicts returns the environment's cached verdicts when they
// are younger than the TTL, or nil.
func loadInventoryVerdicts(cfg *config.Config) *inventoryVerdicts {
	key, ok := verdictCacheKey(cfg)
	if !ok {
		return nil
	}
	var v *inventoryVerdicts
	if runSession != nil {
		sessionVerdictsMu.Lock()
		v = sessionVerdicts[key]
		sessionVerdictsMu.Unlock()
	} else {
		data, err := os.ReadFile(verdictCachePath(key))
		if err != nil {
			return nil
		}
		v = &inventoryVerdicts{}
		if json.Unmarshal(data, v) != nil {
			return nil
		}
	}
	if v == nil || v.age() > inventoryVerdictTTL {
		return nil
	}
	return v
}

// saveInventoryVerdicts caches verdicts for the environment. Best-effort: a
// cache that cannot be written only costs the next listing a structural pass.
func saveInventoryVerdicts(cfg *config.Config, v *inventoryVerdicts) {
	key, ok := verdictCacheKey(cfg)
	if !ok {
		return
	}
	if runSession != nil {
		sessionVerdictsMu.Lock()
		sessionVerdicts[key] = v
		sessionVerdictsMu.Unlock()
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	path := verdictCachePath(key)
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	if os.WriteFile(path+".tmp", data, 0o600) == nil {
		_ = os.Rename(path+".tmp", path)
	}
}

func verdictCachePath(key string) string {
	return filepath.Join(recipeCacheRoot(), "inventory", safeFileName(key)+".json")
}
