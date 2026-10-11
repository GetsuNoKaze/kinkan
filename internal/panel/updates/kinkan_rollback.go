package updates

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"mikan/internal/release"
)

var ErrRollbackUnavailable = errors.New("rollback_unavailable")
var ErrUpdateBusy = errors.New("update_busy")

// Rollback is the host's public description. The authorized Docker image is kept
// separately by root, outside the panel container's filesystem.
type Rollback struct {
	Current   string `json:"current"`
	Version   string `json:"version"`
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	At        string `json:"at"`
}

func (c *Checker) Rollback() Rollback {
	v := Rollback{Current: c.version, Reason: "no_previous"}
	if c.dir == "" {
		return v
	}
	data, err := os.ReadFile(filepath.Join(c.dir, "rollback.json"))
	if err != nil || len(data) > 4096 || json.Unmarshal(data, &v) != nil {
		return Rollback{Current: c.version, Reason: "no_previous"}
	}
	if v.Current != c.version {
		v.Available, v.Reason = false, "version_changed"
	} else if v.Available && (!release.Valid(v.Version) || !release.Newer(c.version, v.Version)) {
		v.Available, v.Reason = false, "version_unsupported"
	}
	return v
}

func (c *Checker) kinkanBusy() error {
	if _, pending := c.Requested(); pending {
		return ErrUpdateBusy
	}
	if host, ok := c.Host(); ok && host.State == "running" {
		return ErrUpdateBusy
	}
	return nil
}

// Busy is a preflight check; requests check again while holding the queue lock.
func (c *Checker) Busy() bool { return c.kinkanBusy() != nil }

func (c *Checker) kinkanRequestUpdate() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.kinkanBusy(); err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]string{"at": c.now().UTC().Format(time.RFC3339)})
	return c.write("request", data)
}

// RequestRollback never accepts an image or a caller-selected target version.
// The host revalidates the record under its update lock before stopping anything.
func (c *Checker) RequestRollback(version string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dir == "" {
		return ErrUnavailable
	}
	if err := c.kinkanBusy(); err != nil {
		return err
	}
	v := c.Rollback()
	if !v.Available || version != v.Version {
		return ErrRollbackUnavailable
	}
	data, _ := json.Marshal(map[string]string{"do": "rollback", "from": c.version, "version": v.Version, "at": c.now().UTC().Format(time.RFC3339)})
	return c.write("request", data)
}
