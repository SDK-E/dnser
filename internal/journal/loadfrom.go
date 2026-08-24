package journal

import (
	"encoding/json"
	"fmt"
	"os"
)

func LoadPlanFrom(path string) (*Plan, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plan %s: %w", path, err)
	}
	var p Plan
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parse plan %s: %w", path, err)
	}
	p.Normalize()
	return &p, nil
}

func SavePlanTo(path string, p *Plan) error {
	p.Normalize()
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plan: %w", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write plan %s: %w", path, err)
	}
	return nil
}
