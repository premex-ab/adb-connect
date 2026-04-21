// Package adb shells out to the system `adb` CLI. All calls use os/exec with
// an explicit argv slice — never the shell-interpolated form.
package adb

import (
	"context"
	"os/exec"
	"strings"
)

type Result struct {
	OK     bool
	Stdout string
	Stderr string
	Code   int
}

type Device struct {
	Serial string
	State  string
}

func run(ctx context.Context, args ...string) Result {
	cmd := exec.CommandContext(ctx, "adb", args...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			return Result{OK: false, Stdout: out.String(), Stderr: errb.String() + err.Error(), Code: -1}
		}
	}
	return Result{OK: code == 0, Stdout: out.String(), Stderr: errb.String(), Code: code}
}

func Pair(ctx context.Context, host string, port int, code string) (Result, error) {
	return run(ctx, "pair", hostPort(host, port), code), nil
}

func Connect(ctx context.Context, host string, port int) (Result, error) {
	return run(ctx, "connect", hostPort(host, port)), nil
}

func Install(ctx context.Context, apkPath string) (Result, error) {
	return run(ctx, "install", "-r", apkPath), nil
}

func GrantWriteSecureSettings(ctx context.Context, pkg string) (Result, error) {
	return run(ctx, "shell", "pm", "grant", pkg, "android.permission.WRITE_SECURE_SETTINGS"), nil
}

func Devices(ctx context.Context) ([]Device, error) {
	r := run(ctx, "devices")
	if !r.OK {
		return nil, nil
	}
	var out []Device
	for i, line := range strings.Split(r.Stdout, "\n") {
		if i == 0 {
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) >= 2 {
			out = append(out, Device{Serial: f[0], State: f[1]})
		}
	}
	return out, nil
}

func hostPort(host string, port int) string {
	// Avoid fmt import bloat.
	buf := make([]byte, 0, len(host)+6)
	buf = append(buf, host...)
	buf = append(buf, ':')
	buf = appendInt(buf, port)
	return string(buf)
}

func appendInt(b []byte, n int) []byte {
	if n == 0 {
		return append(b, '0')
	}
	var tmp [12]byte
	i := len(tmp)
	for n > 0 {
		i--
		tmp[i] = byte('0' + n%10)
		n /= 10
	}
	return append(b, tmp[i:]...)
}
