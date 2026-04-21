package tailscale

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
)

type Status struct {
	BackendState string `json:"BackendState"`
	Self         struct {
		HostName     string   `json:"HostName"`
		DNSName      string   `json:"DNSName"`
		TailscaleIPs []string `json:"TailscaleIPs"`
	} `json:"Self"`
}

func run(ctx context.Context, args ...string) (string, string, int) {
	cmd := exec.CommandContext(ctx, "tailscale", args...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			return out.String(), errb.String() + err.Error(), -1
		}
	}
	return out.String(), errb.String(), code
}

func GetStatus(ctx context.Context) *Status {
	stdout, _, code := run(ctx, "status", "--json")
	if code != 0 {
		return nil
	}
	var s Status
	if err := json.Unmarshal([]byte(stdout), &s); err != nil {
		return nil
	}
	return &s
}

func MagicDNSName(ctx context.Context) string {
	s := GetStatus(ctx)
	if s == nil {
		return ""
	}
	return strings.TrimSuffix(s.Self.DNSName, ".")
}

func IPv4(ctx context.Context) string {
	s := GetStatus(ctx)
	if s == nil {
		return ""
	}
	for _, ip := range s.Self.TailscaleIPs {
		if strings.HasPrefix(ip, "100.") {
			return ip
		}
	}
	return ""
}

func IsInstalled(ctx context.Context) bool {
	_, _, code := run(ctx, "version")
	return code == 0
}

func UpWithAuthKey(ctx context.Context, key string) error {
	_, stderr, code := run(ctx, "up", "--auth-key="+key)
	if code == 0 {
		return nil
	}
	return &exec.ExitError{ProcessState: nil, Stderr: []byte(stderr)}
}
