package detector

import (
	"context"

	"github.com/nzhussup/fleck/internal/graph"
	"github.com/nzhussup/fleck/internal/model"
)

type Metadata struct {
	ID, Name, Description string
	Severity              model.Severity
}

type AnalysisContext struct {
	Facts   *model.Facts
	Graph   *graph.Graph
	Queries graph.Queries
}
type Detector interface {
	Metadata() Metadata
	Requires() []model.Capability
	Detect(context.Context, *AnalysisContext) ([]model.Finding, error)
}
