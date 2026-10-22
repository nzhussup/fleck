package helm

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type Result struct {
	Manifest []byte
	Stderr   string
	Duration time.Duration
}

type Renderer interface {
	Render(context.Context, string, string, []string) (Result, error)
}

type CommandRenderer struct{ Binary string }

func (r CommandRenderer) Render(ctx context.Context, release, chart string, values []string) (Result, error) {
	bin := r.Binary
	if bin == "" {
		bin = "helm"
	}
	args := []string{"template", release, chart}
	for _, v := range values {
		args = append(args, "-f", v)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	err := cmd.Run()
	result := Result{Manifest: stdout.Bytes(), Stderr: stderr.String(), Duration: time.Since(start)}
	if err != nil {
		// Keep stderr available to trusted callers but do not copy arbitrary
		// chart output into the user-facing error, where it could expose values.
		return result, fmt.Errorf("helm template failed: %w", err)
	}
	return result, nil
}
