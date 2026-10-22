package helm

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCommandRendererPassesOrderedArgumentsWithoutShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is for Linux/macOS")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "helm")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := (CommandRenderer{Binary: bin}).Render(context.Background(), "release name", "chart; untouched", []string{"first values.yaml", "second.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	want := "template\nrelease name\nchart; untouched\n-f\nfirst values.yaml\n-f\nsecond.yaml\n"
	if string(r.Manifest) != want {
		t.Fatalf("arguments:\n%s\nwant:\n%s", r.Manifest, want)
	}
}

func TestCommandRendererReturnsStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is for Linux/macOS")
	}
	bin := filepath.Join(t.TempDir(), "helm")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho broken >&2\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := (CommandRenderer{Binary: bin}).Render(context.Background(), "r", "c", nil)
	if err == nil || !strings.Contains(r.Stderr, "broken") || strings.Contains(err.Error(), "broken") {
		t.Fatalf("error=%v", err)
	}
}
