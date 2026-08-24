package journal

import (
	"context"
	"fmt"
	"os"
)

type TrustOperator interface {
	IsTrusted(ctx context.Context, certPath string) (bool, error)
	Trust(ctx context.Context, certPath string) error
	Untrust(ctx context.Context, certPath string) error
}

type CATrustExecutor struct {
	Runner CommandRunner
	Ops    TrustOperator
}

func (c *CATrustExecutor) Capture(ctx context.Context, s *Step) (*Capture, error) {
	cert, err := s.paramStr("cert")
	if err != nil {
		return nil, err
	}
	trusted, err := c.Ops.IsTrusted(ctx, cert)
	if err != nil {
		return nil, fmt.Errorf("probe trust for %s: %w", cert, err)
	}
	return &Capture{CA: &CACapture{CertPath: cert, WasTrust: trusted}}, nil
}

func (c *CATrustExecutor) Apply(ctx context.Context, s *Step) error {
	cert, err := s.paramStr("cert")
	if err != nil {
		return err
	}
	switch s.Kind {
	case KindCATrust:
		return c.Ops.Trust(ctx, cert)
	case KindCAUntrust:
		return c.Ops.Untrust(ctx, cert)
	}
	return fmt.Errorf("CA executor cannot apply kind %q", s.Kind)
}

func (c *CATrustExecutor) Invert(ctx context.Context, s *Step) error {
	cap := s.Capture
	if cap == nil || cap.CA == nil {
		return fmt.Errorf("step %s: no CA capture; refusing to invert blind", s.ID)
	}
	var err error
	if cap.CA.WasTrust {
		err = c.Ops.Trust(ctx, cap.CA.CertPath)
	} else {
		err = c.Ops.Untrust(ctx, cap.CA.CertPath)
	}
	if err != nil {
		return fmt.Errorf("restore CA trust state for %s: %w", cap.CA.CertPath, err)
	}
	return nil
}

func (c *CATrustExecutor) Verify(ctx context.Context, s *Step) error {
	cert, err := s.paramStr("cert")
	if err != nil {
		return err
	}
	trusted, err := c.Ops.IsTrusted(ctx, cert)
	if err != nil {
		return err
	}
	want := s.Kind == KindCATrust
	if trusted != want {
		state := "untrusted"
		if trusted {
			state = "trusted"
		}
		return fmt.Errorf("%s is %s after step", cert, state)
	}
	return nil
}

type ServiceOperator interface {
	IsLoaded(ctx context.Context, name string) (bool, error)
	Load(ctx context.Context, defPath string) error
	Unload(ctx context.Context, name string) error
}

type ServiceExecutor struct {
	Ops ServiceOperator
}

func serviceTarget(params map[string]any) (string, error) {
	v, ok := params["target"].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("missing param target")
	}
	return v, nil
}

func serviceName(s *Step) (string, error) {
	name, err := s.paramStr("name")
	if err != nil {
		return "", err
	}
	return name, nil
}

func (sv *ServiceExecutor) Capture(ctx context.Context, s *Step) (*Capture, error) {
	name, err := serviceName(s)
	if err != nil {
		return nil, err
	}
	target, _ := serviceTarget(s.Params)
	loaded, err := sv.Ops.IsLoaded(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("probe service %s: %w", name, err)
	}
	defPath := ""
	if v, ok := s.Params["def"].(string); ok {
		defPath = v
	}
	return &Capture{Service: &ServiceCapture{Name: name, WasLoaded: loaded, DefPath: defPath, TargetPath: target}}, nil
}

func (sv *ServiceExecutor) Apply(ctx context.Context, s *Step) error {
	name, err := serviceName(s)
	if err != nil {
		return err
	}
	switch s.Kind {
	case KindServiceInstal:
		target, err := serviceTarget(s.Params)
		if err != nil {
			return err
		}
		def, err := s.paramStr("def")
		if err != nil {
			return err
		}
		data, err := os.ReadFile(def)
		if err != nil {
			return fmt.Errorf("read service def %s: %w", def, err)
		}
		if err := os.MkdirAll(parentDir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", target, err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("install %s: %w", target, err)
		}
		return sv.Ops.Load(ctx, target)
	case KindServiceRemove:
		if loaded, lerr := sv.Ops.IsLoaded(ctx, name); lerr == nil && loaded {
			if err := sv.Ops.Unload(ctx, name); err != nil {
				return err
			}
		}
		target, terr := serviceTarget(s.Params)
		if terr != nil {
			return nil
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", target, err)
		}
		return nil
	}
	return fmt.Errorf("service executor cannot apply kind %q", s.Kind)
}

func (sv *ServiceExecutor) Invert(ctx context.Context, s *Step) error {
	cap := s.Capture
	if cap == nil || cap.Service == nil {
		return fmt.Errorf("step %s: no service capture; refusing to invert blind", s.ID)
	}
	sc := cap.Service
	if sc.WasLoaded {
		if sc.TargetPath != "" {
			if _, err := os.Stat(sc.TargetPath); err == nil {
				if err := sv.Ops.Load(ctx, sc.TargetPath); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if loaded, lerr := sv.Ops.IsLoaded(ctx, sc.Name); lerr == nil && loaded {
		if err := sv.Ops.Unload(ctx, sc.Name); err != nil {
			return err
		}
	}
	if sc.TargetPath != "" {
		if err := os.Remove(sc.TargetPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", sc.TargetPath, err)
		}
	}
	return nil
}

func (sv *ServiceExecutor) Verify(ctx context.Context, s *Step) error {
	name, err := serviceName(s)
	if err != nil {
		return err
	}
	loaded, err := sv.Ops.IsLoaded(ctx, name)
	if err != nil {
		return err
	}
	want := s.Kind == KindServiceInstal
	if loaded != want {
		state := "not loaded"
		if loaded {
			state = "loaded"
		}
		return fmt.Errorf("service %s is %s after step", name, state)
	}
	return nil
}
