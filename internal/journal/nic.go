package journal

import (
	"context"
	"fmt"
	"strings"
)

type NICExecutor struct {
	Runner CommandRunner
}

func (n *NICExecutor) currentDNS(ctx context.Context, device string) ([]string, bool, error) {
	out, err := n.Runner.Run(ctx, "networksetup", "-getdnsservers", device)
	if err != nil {
		if strings.Contains(out, "aren't any DNS Servers") {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("read DNS for %s: %w (out=%q)", device, err, strings.TrimSpace(out))
	}
	servers := append([]string(nil), strings.Fields(out)...)
	return servers, false, nil
}

func (n *NICExecutor) Capture(ctx context.Context, s *Step) (*Capture, error) {
	device, err := s.paramStr("device")
	if err != nil {
		return nil, err
	}
	servers, unset, err := n.currentDNS(ctx, device)
	if err != nil {
		return nil, err
	}
	return &Capture{NICDNS: &NICDNSCapture{Device: device, WasUnset: unset, Servers: servers}}, nil
}

func (n *NICExecutor) Apply(ctx context.Context, s *Step) error {
	device, err := s.paramStr("device")
	if err != nil {
		return err
	}
	raw, ok := s.Params["servers"].([]any)
	if !ok {
		return fmt.Errorf("step %s: param servers must be a list", s.ID)
	}
	args := []string{"-setdnsservers", device}
	for _, v := range raw {
		str, ok := v.(string)
		if !ok {
			return fmt.Errorf("step %s: server entry not a string", s.ID)
		}
		args = append(args, str)
	}
	if len(raw) == 0 {
		args = append(args, "Empty")
	}
	if _, err := n.Runner.Run(ctx, "networksetup", args...); err != nil {
		return err
	}
	return nil
}

func (n *NICExecutor) Invert(ctx context.Context, s *Step) error {
	cap := s.Capture
	if cap == nil || cap.NICDNS == nil {
		return fmt.Errorf("step %s: no NIC capture; refusing to invert blind", s.ID)
	}
	args := []string{"-setdnsservers", cap.NICDNS.Device}
	if cap.NICDNS.WasUnset {
		args = append(args, "Empty")
	} else {
		args = append(args, cap.NICDNS.Servers...)
	}
	if _, err := n.Runner.Run(ctx, "networksetup", args...); err != nil {
		return err
	}
	return nil
}

func (n *NICExecutor) Verify(ctx context.Context, s *Step) error {
	device, err := s.paramStr("device")
	if err != nil {
		return err
	}
	wantAny := s.Params["servers"]
	raw, _ := wantAny.([]any)
	got, gotUnset, err := n.currentDNS(ctx, device)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		if !gotUnset && len(got) > 0 {
			return fmt.Errorf("%s still has DNS servers %v after clearing", device, got)
		}
		return nil
	}
	if gotUnset {
		return fmt.Errorf("%s has no DNS servers after set", device)
	}
	if len(got) != len(raw) {
		return fmt.Errorf("%s server count mismatch: got %v", device, got)
	}
	for i := range raw {
		if str, _ := raw[i].(string); str != got[i] {
			return fmt.Errorf("%s server %d: got %s want %s", device, i, got[i], str)
		}
	}
	return nil
}
