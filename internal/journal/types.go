package journal

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrPlanNotFound   = errors.New("plan not found in journal")
	ErrStepNotApplied = errors.New("step not applied")
)

type StepStatus string

const (
	StatusPending  StepStatus = "pending"
	StatusInflight StepStatus = "inflight"
	StatusApplied  StepStatus = "applied"
	StatusFailed   StepStatus = "failed"
	StatusReversed StepStatus = "reversed"
)

type StepKind string

const (
	KindFileWrite     StepKind = "file_write"
	KindFileRemove    StepKind = "file_remove"
	KindNICDNSSet     StepKind = "nic_dns_set"
	KindCATrust       StepKind = "ca_trust"
	KindCAUntrust     StepKind = "ca_untrust"
	KindServiceInstal StepKind = "service_install"
	KindServiceRemove StepKind = "service_remove"
)

type FileCapture struct {
	Path    string `json:"path"`
	Existed bool   `json:"existed"`
	Perm    uint32 `json:"perm,omitempty"`
	Content string `json:"content,omitempty"`
}

type NICDNSCapture struct {
	Device   string   `json:"device"`
	WasUnset bool     `json:"was_unset"`
	Servers  []string `json:"servers,omitempty"`
}

type ServiceCapture struct {
	Name       string `json:"name"`
	WasLoaded  bool   `json:"was_loaded"`
	DefPath    string `json:"def_path,omitempty"`
	TargetPath string `json:"target_path,omitempty"`
}

type CACapture struct {
	CertPath string `json:"cert_path"`
	WasTrust bool   `json:"was_trust"`
}

type Capture struct {
	File    *FileCapture    `json:"file,omitempty"`
	NICDNS  *NICDNSCapture  `json:"nicdns,omitempty"`
	Service *ServiceCapture `json:"service,omitempty"`
	CA      *CACapture      `json:"ca,omitempty"`
}

type Step struct {
	ID      string         `json:"id"`
	Kind    StepKind       `json:"kind"`
	Params  map[string]any `json:"params"`
	Capture *Capture       `json:"capture,omitempty"`
	Status  StepStatus     `json:"status"`
	Error   string         `json:"error,omitempty"`
	DoneAt  *time.Time     `json:"done_at,omitempty"`
}

type Plan struct {
	ID        string    `json:"id"`
	Intent    string    `json:"intent"`
	CreatedAt time.Time `json:"created_at"`
	Steps     []*Step   `json:"steps"`
	Status    StepStatus
}

func (p *Plan) Normalize() {
	p.Status = planStatus(p)
}

func planStatus(p *Plan) StepStatus {
	if len(p.Steps) == 0 {
		return StatusApplied
	}
	allReversed := true
	for _, s := range p.Steps {
		switch s.Status {
		case StatusFailed:
			return StatusFailed
		case StatusPending, StatusInflight:
			return StatusPending
		case StatusApplied:
			allReversed = false
		}
	}
	if allReversed {
		return StatusReversed
	}
	return StatusApplied
}

func (s *Step) paramStr(key string) (string, error) {
	v, ok := s.Params[key]
	if !ok {
		return "", fmt.Errorf("step %s (%s): missing param %q", s.ID, s.Kind, key)
	}
	str, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("step %s (%s): param %q is not a string", s.ID, s.Kind, key)
	}
	return str, nil
}

func (c *Capture) Marshal() (json.RawMessage, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("marshal capture: %w", err)
	}
	return b, nil
}
