package tailscale_test

import (
	"context"
	"testing"

	"github.com/premex-ab/adb-connect/internal/tailscale"
	"github.com/premex-ab/adb-connect/internal/testutil"
)

func TestIsInstalledTrue(t *testing.T) {
	testutil.FakeBinary(t, "tailscale", `echo "1.62.0"`)
	if !tailscale.IsInstalled(context.Background()) {
		t.Fatal("expected installed")
	}
}

func TestIsInstalledFalse(t *testing.T) {
	testutil.FakeBinary(t, "tailscale", `exit 1`)
	if tailscale.IsInstalled(context.Background()) {
		t.Fatal("expected not installed")
	}
}

func TestIPv4ParsesTailscaleIP(t *testing.T) {
	testutil.FakeBinary(t, "tailscale", `printf '{"BackendState":"Running","Self":{"HostName":"myhost","DNSName":"myhost.ts.net.","TailscaleIPs":["100.64.0.1","fd7a::1"]}}'`)
	ip := tailscale.IPv4(context.Background())
	if ip != "100.64.0.1" {
		t.Fatalf("got %q, want 100.64.0.1", ip)
	}
}

func TestMagicDNSNameTrimsTrailingDot(t *testing.T) {
	testutil.FakeBinary(t, "tailscale", `printf '{"BackendState":"Running","Self":{"HostName":"myhost","DNSName":"myhost.ts.net.","TailscaleIPs":["100.64.0.1"]}}'`)
	name := tailscale.MagicDNSName(context.Background())
	if name != "myhost.ts.net" {
		t.Fatalf("got %q, want myhost.ts.net", name)
	}
}
