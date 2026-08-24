package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Store struct {
	Dir string
}

func OpenStore(rootDir string) (*Store, error) {
	dir := filepath.Join(rootDir, ".dnser", "journal")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create journal dir: %w", err)
	}
	return &Store{Dir: dir}, nil
}

func (st *Store) planPath(id string) string {
	return filepath.Join(st.Dir, id+".json")
}

func NewPlan(intent string) *Plan {
	return &Plan{
		ID:        time.Now().UTC().Format("20060102-150405") + "-" + uuid.NewString()[:8],
		Intent:    intent,
		CreatedAt: time.Now().UTC(),
	}
}

func (st *Store) Save(p *Plan) error {
	p.Normalize()
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plan %s: %w", p.ID, err)
	}
	tmp, err := os.CreateTemp(st.Dir, ".plan-*")
	if err != nil {
		return fmt.Errorf("journal temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write plan %s: %w", p.ID, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsync plan %s: %w", p.ID, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close plan %s: %w", p.ID, err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("chmod plan %s: %w", p.ID, err)
	}
	if err := os.Rename(tmpName, st.planPath(p.ID)); err != nil {
		return fmt.Errorf("swap plan %s: %w", p.ID, err)
	}
	if err := syncDir(st.Dir); err != nil {
		return fmt.Errorf("fsync journal dir: %w", err)
	}
	return nil
}

func (st *Store) Load(id string) (*Plan, error) {
	if strings.Contains(id, "/") || strings.Contains(id, "..") {
		return nil, fmt.Errorf("invalid plan id %q", id)
	}
	b, err := os.ReadFile(st.planPath(id))
	if os.IsNotExist(err) {
		return nil, ErrPlanNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read plan %s: %w", id, err)
	}
	var p Plan
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parse plan %s: %w", id, err)
	}
	p.Normalize()
	return &p, nil
}

func (st *Store) List() ([]*Plan, error) {
	entries, err := os.ReadDir(st.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read journal dir: %w", err)
	}
	var ids []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, ".") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(name, ".json"))
	}
	sort.Strings(ids)
	plans := make([]*Plan, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		p, err := st.Load(ids[i])
		if err != nil {
			continue
		}
		plans = append(plans, p)
	}
	return plans, nil
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}
