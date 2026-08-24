package helper

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed servicedefs/enterprises.sdk.dnser.plist
var launchdTmpl string

//go:embed servicedefs/dnser.service
var systemdTmpl string

type ServiceVars struct {
	BinPath  string
	LogsDir  string
	StateDir string
}

func RenderServiceDef(goos string, v ServiceVars) (string, error) {
	r := strings.NewReplacer(
		"{bin_path}", v.BinPath,
		"{logs_dir}", v.LogsDir,
		"{state_dir}", v.StateDir,
	)
	switch goos {
	case "darwin":
		return r.Replace(launchdTmpl), nil
	case "linux":
		return r.Replace(systemdTmpl), nil
	default:
		return "", fmt.Errorf("no service definition for %s", goos)
	}
}

func ServiceLabel() string {
	return "enterprises.sdk.dnser"
}
