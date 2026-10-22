package cfg006

import (
	"context"
	"testing"

	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/graph"
	"github.com/nzhussup/fleck/internal/model"
)

func TestDetectsStructuredInvalidReferences(t *testing.T) {
	facts := &model.Facts{InvalidReferences: []model.InvalidReference{{Kind: "environment-key", Subject: "TOKEN", Reference: "default/config:token", Message: "mandatory referenced key is missing", Location: model.SourceLocation{File: "manifest.yaml", Line: 12}}}}
	findings, err := New().Detect(context.Background(), &detector.AnalysisContext{Facts: facts, Queries: graph.Queries{Facts: facts}})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].RuleID != "CFG006" || findings[0].Severity != model.SeverityError {
		t.Fatalf("findings=%#v", findings)
	}
}
