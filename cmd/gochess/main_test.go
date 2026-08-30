package main

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// capture redirects os.Stdout for the duration of fn and returns what was
// written. The CLI helpers print directly to stdout, so this is the only way to
// exercise them.
func capture(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}

func TestRunBench(t *testing.T) {
	out := capture(t, runBench)
	if !strings.Contains(out, "bench:") || !strings.Contains(out, "nps)") {
		t.Errorf("bench output missing summary line:\n%s", out)
	}
	if strings.Count(out, "bestmove") < 4 {
		t.Errorf("expected one line per bench position:\n%s", out)
	}
}

func TestRunPerftDefaultAndExplicit(t *testing.T) {
	if out := capture(t, func() { runPerft(nil) }); !strings.Contains(out, "nps") {
		t.Errorf("default perft produced no series:\n%s", out)
	}
	out := capture(t, func() {
		runPerft([]string{"2", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1"})
	})
	if !strings.Contains(out, "depth  1") || !strings.Contains(out, "depth  2") {
		t.Errorf("perft series incomplete:\n%s", out)
	}
}

func TestRunPerftRejectsBadDepth(t *testing.T) {
	if os.Getenv("GOCHESS_BAD_DEPTH") == "1" {
		runPerft([]string{"notanumber"})
		return
	}
	// runPerft calls os.Exit(2) on a bad depth; run it in a subprocess.
	out, err := runSelf(t, "TestRunPerftRejectsBadDepth", "GOCHESS_BAD_DEPTH=1")
	if err == nil {
		t.Fatalf("expected a non-zero exit, output:\n%s", out)
	}
	if !strings.Contains(out, "invalid depth") {
		t.Errorf("missing diagnostic:\n%s", out)
	}
}

func TestMainDispatch(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"version"}, "gochess "},
		{[]string{"help"}, "usage: gochess"},
	} {
		out := capture(t, func() {
			os.Args = append([]string{"gochess"}, tc.args...)
			main()
		})
		if !strings.Contains(out, tc.want) {
			t.Errorf("main %v: output missing %q:\n%s", tc.args, tc.want, out)
		}
	}
}

func TestMainSpeaksUCIOnStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origIn := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origIn }()
	go func() {
		_, _ = io.WriteString(w, "uci\nquit\n")
		_ = w.Close()
	}()

	out := capture(t, func() {
		os.Args = []string{"gochess"} // no subcommand -> UCI mode
		main()
	})
	if !strings.Contains(out, "uciok") {
		t.Errorf("no-arg main did not speak UCI:\n%s", out)
	}
}

func TestMainUnknownCommandExits(t *testing.T) {
	if os.Getenv("GOCHESS_UNKNOWN") == "1" {
		os.Args = []string{"gochess", "bogus"}
		main()
		return
	}
	out, err := runSelf(t, "TestMainUnknownCommandExits", "GOCHESS_UNKNOWN=1")
	if err == nil {
		t.Fatalf("expected non-zero exit for unknown command:\n%s", out)
	}
	if !strings.Contains(out, "unknown command") {
		t.Errorf("missing diagnostic:\n%s", out)
	}
}

// runSelf re-executes this test binary running only testName, with extra env
// set, and returns the combined output. Used to observe os.Exit paths.
func runSelf(t *testing.T, testName, env string) (string, error) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := exec.Command(exe, "-test.run=^"+testName+"$")
	cmd.Env = append(os.Environ(), env)
	b, err := cmd.CombinedOutput()
	return string(b), err
}
