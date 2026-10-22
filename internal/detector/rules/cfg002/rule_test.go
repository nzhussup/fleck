package cfg002

import (
	"context"
	"testing"

	"github.com/nzhussup/fleck/internal/detector"
	"github.com/nzhussup/fleck/internal/graph"
	"github.com/nzhussup/fleck/internal/model"
)

func TestSimilarity(t *testing.T) {
	if got := similarity("PAYMENT_API_URL", "PAYMENTS_API_URL"); got < DefaultThreshold {
		t.Fatalf("similarity=%f", got)
	}
	if got := similarity("DATABASE_URL", "ENABLE_METRICS"); got >= DefaultThreshold {
		t.Fatalf("unrelated similarity=%f", got)
	}
}

func TestExactUnverifiedDeclarationSuppressesTypoSuggestion(t *testing.T) {
	facts := &model.Facts{
		EnvReads: []model.EnvRead{{Name: "REDIS_ADDRESS", Resolved: true}},
		Container: model.Container{Supplies: []model.EnvSupply{
			{Name: "REDIS_ADRESS", State: model.SupplyDefinite},
			{Name: "REDIS_ADDRESS", State: model.SupplyUnverified},
		}},
	}
	queries := graph.Queries{Facts: facts}
	findings, err := New(DefaultThreshold).Detect(context.Background(), &detector.AnalysisContext{Facts: facts, Queries: queries})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings=%#v, want exact unverified declaration to suppress typo heuristic", findings)
	}
}
