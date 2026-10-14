package uci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dyanis-chess/internal/nnue"
	"dyanis-chess/internal/search"
)

func TestEvalFileOption(t *testing.T) {
	defer search.SetNetwork(nil)

	n, err := nnue.New(16, nnue.CReLU, 255, 64, 400)
	if err != nil {
		t.Fatal(err)
	}
	n.B2[0] = 12345
	data, err := n.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	good := filepath.Join(dir, "my net.bin") // a space in the path, on purpose
	if err := os.WriteFile(good, data, 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.bin")
	if err := os.WriteFile(bad, data[:100], 0o644); err != nil {
		t.Fatal(err)
	}

	if got := run(t, "uci\nquit\n", 3, 1, nil); !strings.Contains(got, "option name EvalFile type string") {
		t.Fatalf("EvalFile not advertised:\n%s", got)
	}

	got := run(t, "setoption name EvalFile value "+good+"\nposition startpos\ngo depth 2\nquit\n", 3, 1, nil)
	if !strings.Contains(got, "info string EvalFile: "+good+", hidden 16, CReLU") || !strings.Contains(got, "bestmove") {
		t.Fatalf("loading a good file:\n%s", got)
	}
	if search.Network() == nil {
		t.Fatal("network not set")
	}

	got = run(t, "setoption name EvalFile value "+bad+"\nquit\n", 3, 1, nil)
	if !strings.Contains(got, "info string EvalFile:") || search.Network() == nil {
		t.Fatalf("a bad file must report an error and keep the loaded network:\n%s", got)
	}

	run(t, "setoption name EvalFile value <empty>\nquit\n", 3, 1, nil)
	if search.Network() != nil {
		t.Fatal("<empty> must switch back to the hand-written eval")
	}
}
