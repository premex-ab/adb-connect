package pair

import (
	"context"
	"errors"
	"testing"
	"time"
)

// newTestConfig returns a Config wired with the supplied seam functions so that
// no real mDNS, adb, or browser I/O is performed.
func newTestConfig(
	browsePairFn func(ctx context.Context, serviceName string) (<-chan browseResult, error),
	browseConnectFn func(ctx context.Context, peerHost string) (<-chan browseResult, error),
	runAdbPairFn func(ctx context.Context, host string, port int, code string) error,
	runAdbConnectFn func(ctx context.Context, host string, port int) error,
) Config {
	return Config{
		Open:          nil, // terminal mode — no browser, no HTTP server
		Timeout:       10 * time.Second,
		Logger:        nil,
		browsePair:    browsePairFn,
		browseConnect: browseConnectFn,
		runAdbPair:    runAdbPairFn,
		runAdbConnect: runAdbConnectFn,
	}
}

// TestRun_Success verifies that when mDNS browse returns a valid entry,
// adb pair and connect are called and Run returns "<ip>:<port>".
func TestRun_Success(t *testing.T) {
	const fakeHost = "10.0.1.42"
	const fakePairPort = 37001
	const fakeConnPort = 37002

	cfg := newTestConfig(
		// browsePair: return one entry matching any service name
		func(ctx context.Context, serviceName string) (<-chan browseResult, error) {
			ch := make(chan browseResult, 1)
			ch <- browseResult{Host: fakeHost, Port: fakePairPort}
			return ch, nil
		},
		// browseConnect: return one entry with the same host
		func(ctx context.Context, peerHost string) (<-chan browseResult, error) {
			ch := make(chan browseResult, 1)
			ch <- browseResult{Host: peerHost, Port: fakeConnPort}
			return ch, nil
		},
		// runAdbPair: succeed
		func(ctx context.Context, host string, port int, code string) error {
			return nil
		},
		// runAdbConnect: succeed
		func(ctx context.Context, host string, port int) error {
			return nil
		},
	)

	addr, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	want := fakeHost + ":37002"
	if addr != want {
		t.Errorf("Run() addr = %q, want %q", addr, want)
	}
}

// TestRun_PairFailure verifies that a failing adb pair surfaces an error
// that mentions "adb pair".
func TestRun_PairFailure(t *testing.T) {
	cfg := newTestConfig(
		func(ctx context.Context, serviceName string) (<-chan browseResult, error) {
			ch := make(chan browseResult, 1)
			ch <- browseResult{Host: "10.0.0.1", Port: 38000}
			return ch, nil
		},
		nil, // browseConnect should not be reached
		func(ctx context.Context, host string, port int, code string) error {
			return errors.New("device rejected pairing code")
		},
		nil,
	)

	_, err := Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	if !containsSubstring(err.Error(), "adb pair") {
		t.Errorf("error %q does not mention 'adb pair'", err.Error())
	}
}

// TestRun_ConnectFailure verifies that when pair succeeds but connect fails,
// the error mentions "adb connect".
func TestRun_ConnectFailure(t *testing.T) {
	cfg := newTestConfig(
		func(ctx context.Context, serviceName string) (<-chan browseResult, error) {
			ch := make(chan browseResult, 1)
			ch <- browseResult{Host: "10.0.0.2", Port: 38001}
			return ch, nil
		},
		func(ctx context.Context, peerHost string) (<-chan browseResult, error) {
			ch := make(chan browseResult, 1)
			ch <- browseResult{Host: peerHost, Port: 38002}
			return ch, nil
		},
		func(ctx context.Context, host string, port int, code string) error {
			return nil // pair succeeds
		},
		func(ctx context.Context, host string, port int) error {
			return errors.New("connection refused")
		},
	)

	_, err := Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	if !containsSubstring(err.Error(), "adb connect") {
		t.Errorf("error %q does not mention 'adb connect'", err.Error())
	}
}

// TestRun_Timeout verifies that Run returns a deadline-exceeded style error
// when the mDNS browse never delivers an entry before the timeout.
func TestRun_Timeout(t *testing.T) {
	cfg := newTestConfig(
		// browsePair: return a channel that is never written to
		func(ctx context.Context, serviceName string) (<-chan browseResult, error) {
			ch := make(chan browseResult) // unbuffered, never sent to
			go func() {
				// drain when context cancels so the goroutine exits
				<-ctx.Done()
				close(ch)
			}()
			return ch, nil
		},
		nil,
		nil,
		nil,
	)
	cfg.Timeout = 150 * time.Millisecond // very short for test speed

	start := time.Now()
	_, err := Run(context.Background(), cfg)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Run() expected timeout error, got nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("Run() took too long (%v); expected timeout around 150ms", elapsed)
	}
	// The error should wrap context.DeadlineExceeded
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Run() error = %v; want wrapping context.DeadlineExceeded", err)
	}
}

// containsSubstring reports whether s contains sub.
func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && findSubstring(s, sub)
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
