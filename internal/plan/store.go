package plan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Store is the on-disk plan: a single JSON file in the user's own
// data folder. The roster it holds never leaves the machine.
type Store struct {
	Path string // full path of the JSON data file
}

type fileFormat struct {
	Version int   `json:"version"`
	Plan    *Plan `json:"plan"`
}

// DataDir returns HataBan's data directory, honouring
// HATABAN_DATA_DIR, then the platform convention via
// os.UserConfigDir (%APPDATA%\HataBan on Windows,
// ~/Library/Application Support/HataBan on macOS,
// $XDG_CONFIG_HOME/HataBan or ~/.config/HataBan elsewhere).
func DataDir() (string, error) {
	if d := os.Getenv("HATABAN_DATA_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "HataBan"), nil
}

// OpenStore resolves the store location and ensures it exists.
func OpenStore() (*Store, error) {
	dir, err := DataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{Path: filepath.Join(dir, "hataban.json")}, nil
}

// Load reads the plan. A missing file is an empty plan, not an error.
func (s *Store) Load() (*Plan, error) {
	raw, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		p := &Plan{}
		p.Normalize()
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		p := &Plan{}
		p.Normalize()
		return p, nil
	}
	var f fileFormat
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("data file is corrupted: %w", err)
	}
	if f.Plan == nil {
		f.Plan = &Plan{}
	}
	f.Plan.Normalize()
	return f.Plan, nil
}

// Save writes the plan atomically (temp file + rename), so a crash or
// a power cut mid-write can never truncate the roster or the table.
func (s *Store) Save(p *Plan) error {
	raw, err := json.MarshalIndent(fileFormat{Version: 1, Plan: p}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}
