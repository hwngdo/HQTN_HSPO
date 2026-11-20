package diagnostics

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

var caseIDPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}_[0-9a-f]{24}$`)
var dayPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// List returns bounded diagnostic metadata only. It never reads or returns the
// source PDF or logs. Sorting and truncation are applied after scanning so a
// filesystem's iteration order cannot hide newer records.
func (s *Store) List(limit int) ([]Event, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Event, 0)
	days, err := os.ReadDir(s.Root)
	if errors.Is(err, os.ErrNotExist) {
		return []Event{}, nil
	}
	if err != nil {
		return nil, err
	}
	for _, day := range days {
		if !day.IsDir() || !dayPattern.MatchString(day.Name()) {
			continue
		}
		d, err := time.Parse("2006-01-02", day.Name())
		if err != nil || d.Format("2006-01-02") != day.Name() {
			continue
		}
		cases, err := os.ReadDir(filepath.Join(s.Root, day.Name()))
		if err != nil {
			return nil, err
		}
		for _, c := range cases {
			if !c.IsDir() || !caseIDPattern.MatchString(c.Name()) {
				continue
			}
			if c.Name()[:8] != d.Format("20060102") {
				continue
			}
			p := filepath.Join(s.Root, day.Name(), c.Name(), "error.json")
			info, err := os.Lstat(p)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 32768 {
				continue
			}
			f, err := os.Open(p)
			if err != nil {
				continue
			}
			var ev Event
			err = json.NewDecoder(io.LimitReader(f, 32768)).Decode(&ev)
			_ = f.Close()
			if err != nil || ev.ID != c.Name() {
				continue
			}
			// No arbitrary attacker-controlled fields are used as filenames.
			// A valid metadata file can contain only bounded, redacted strings.
			if len(ev.Username) > 256 || len(ev.Message) > 4096 || len(ev.Filename) > 128 {
				continue
			}
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		ti, ei := time.Parse(time.RFC3339Nano, out[i].Time)
		tj, ej := time.Parse(time.RFC3339Nano, out[j].Time)
		if ei == nil && ej == nil && !ti.Equal(tj) {
			return ti.After(tj)
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
