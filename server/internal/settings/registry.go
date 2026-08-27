package settings

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

type FieldError struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

type entry struct {
	defaultJSON json.RawMessage
	validate    func(json.RawMessage) error
}

var (
	registry = map[string]entry{}
	keyOrder []string
)

func Keys() []string {
	keys := make([]string, len(keyOrder))
	copy(keys, keyOrder)
	return keys
}

func DefaultJSON(key string) (json.RawMessage, bool) {
	e, ok := registry[key]
	if !ok {
		return nil, false
	}
	return append(json.RawMessage(nil), e.defaultJSON...), true
}

func Validate(input map[string]json.RawMessage) []FieldError {
	var errs []FieldError
	for key, raw := range input {
		e, ok := registry[key]
		if !ok {
			errs = append(errs, FieldError{Key: key, Message: "unknown setting"})
			continue
		}
		if err := e.validate(raw); err != nil {
			errs = append(errs, FieldError{Key: key, Message: err.Error()})
		}
	}
	return errs
}

func mustRegister(key string, defaultJSON json.RawMessage, validate func(json.RawMessage) error) {
	if _, exists := registry[key]; exists {
		panic(fmt.Sprintf("duplicate settings key: %s", key))
	}
	registry[key] = entry{
		defaultJSON: append(json.RawMessage(nil), defaultJSON...),
		validate:    validate,
	}
	keyOrder = append(keyOrder, key)
}

func validateIntMin(min int) func(json.RawMessage) error {
	return func(raw json.RawMessage) error {
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return fmt.Errorf("must be a number")
		}
		if math.Trunc(f) != f {
			return fmt.Errorf("must be an integer")
		}
		n := int(f)
		if n < min {
			return fmt.Errorf("must be at least %d", min)
		}
		return nil
	}
}

func validateHealthThreshold(raw json.RawMessage) error {
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return fmt.Errorf("must be a number")
	}
	if f <= 0 || f > 1 {
		return fmt.Errorf("must be greater than 0 and at most 1")
	}
	return nil
}

func validateRetention(raw json.RawMessage) error {
	var rp database.RetentionPolicy
	if err := json.Unmarshal(raw, &rp); err != nil {
		return fmt.Errorf("must be a retention object")
	}
	checks := []struct {
		name string
		val  int
	}{
		{"keep_last", rp.KeepLast},
		{"keep_hourly", rp.KeepHourly},
		{"keep_daily", rp.KeepDaily},
		{"keep_weekly", rp.KeepWeekly},
		{"keep_monthly", rp.KeepMonthly},
		{"keep_yearly", rp.KeepYearly},
	}
	for _, c := range checks {
		if c.val < 0 {
			return fmt.Errorf("%s must be at least 0", c.name)
		}
	}
	return nil
}

func validateBlockedPaths(raw json.RawMessage) error {
	var paths []string
	if err := json.Unmarshal(raw, &paths); err != nil {
		return fmt.Errorf("must be a string array")
	}
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			return fmt.Errorf("paths must be non-empty strings")
		}
	}
	return nil
}

func init() {
	mustRegister("default_retention", json.RawMessage(`{"keep_last":5,"keep_hourly":0,"keep_daily":0,"keep_weekly":0,"keep_monthly":0,"keep_yearly":0}`), validateRetention)
	mustRegister("heartbeat_interval_seconds", json.RawMessage(`30`), validateIntMin(5))
	mustRegister("agent_offline_threshold_seconds", json.RawMessage(`300`), validateIntMin(30))
	mustRegister("job_history_days", json.RawMessage(`30`), validateIntMin(1))
	mustRegister("health_threshold_failing", json.RawMessage(`0.9`), validateHealthThreshold)
	mustRegister("health_threshold_warning", json.RawMessage(`0.99`), validateHealthThreshold)
	mustRegister("max_heatmap_runs", json.RawMessage(`30`), validateIntMin(5))
	mustRegister("default_hook_timeout_seconds", json.RawMessage(`60`), validateIntMin(5))
	mustRegister("file_browser_blocked_paths", json.RawMessage(`["/proc","/sys","/dev","/run/credentials","/selinux","/cgroup"]`), validateBlockedPaths)
	mustRegister("command_timeout_backup_seconds", json.RawMessage(`86400`), validateIntMin(60))
	mustRegister("command_timeout_restore_seconds", json.RawMessage(`86400`), validateIntMin(60))
	mustRegister("command_timeout_list_snapshots_seconds", json.RawMessage(`300`), validateIntMin(5))
	mustRegister("command_timeout_browse_snapshot_seconds", json.RawMessage(`300`), validateIntMin(5))
	mustRegister("command_timeout_browse_filesystem_seconds", json.RawMessage(`30`), validateIntMin(1))
	mustRegister("command_timeout_default_seconds", json.RawMessage(`300`), validateIntMin(5))
	mustRegister("outbox_spill_max_rows", json.RawMessage(`20000`), validateIntMin(100))
	mustRegister("outbox_spill_retention_seconds", json.RawMessage(`604800`), validateIntMin(60))
	mustRegister("outbox_flush_interval_seconds", json.RawMessage(`60`), validateIntMin(1))
	mustRegister("outbox_delivery_timeout_seconds", json.RawMessage(`10`), validateIntMin(1))
	mustRegister("outbox_max_attempts", json.RawMessage(`10`), validateIntMin(1))
}
