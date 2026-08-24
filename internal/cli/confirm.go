package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const (
	SeverityModerate = "moderate"
	SeveritySevere   = "severe"
)

type Change struct {
	Action string `json:"action"`
	Path   string `json:"path,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type ConfirmPlan struct {
	Command  string
	Severity string
	Changes  []Change
}

type ConfirmDecision struct {
	Proceed bool
}

func EvaluateConfirm(o *Output, plan ConfirmPlan, yesFlag bool, confirmFlag, noInput string) error {
	if plan.Severity == "" {
		return nil
	}
	if plan.Severity == SeveritySevere {
		token := ConfirmTokenFor(plan)
		if confirmFlag != token {
			return &ConfirmRequiredError{
				Severity: SeveritySevere,
				Changes:  plan.Changes,
				Hint:     fmt.Sprintf("severe mutation requires --confirm %q", token),
				Token:    token,
			}
		}
		return nil
	}
	if yesFlag {
		return nil
	}
	return &ConfirmRequiredError{
		Severity: SeverityModerate,
		Changes:  plan.Changes,
		Hint:     "moderate mutation requires confirmation",
	}
}

func ConfirmTokenFor(plan ConfirmPlan) string {
	if len(plan.Changes) == 0 {
		return ""
	}
	for _, c := range plan.Changes {
		if c.Action == "purge_project" {
			return strings.TrimPrefix(c.Path, ".dnser/")
		}
	}
	return plan.Command
}

func RenderPlanText(o *Output, plan ConfirmPlan) {
	fmt.Fprintf(o.Stderr, "plan (%s):\n", plan.Severity)
	for _, c := range plan.Changes {
		line := "  " + c.Action
		if c.Path != "" {
			line += " " + c.Path
		}
		if c.Detail != "" {
			line += " — " + c.Detail
		}
		fmt.Fprintln(o.Stderr, line)
	}
}

func PromptTTY(o *Output, question string) (bool, error) {
	if !IsTTY(os.Stdin) {
		return false, fmt.Errorf("cannot prompt without a tty")
	}
	fmt.Fprintf(o.Stderr, "%s [y/N] ", question)
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}
