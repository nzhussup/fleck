package cfg002

import "testing"

func TestSimilarity(t *testing.T) {
	if got := similarity("PAYMENT_API_URL", "PAYMENTS_API_URL"); got < DefaultThreshold {
		t.Fatalf("similarity=%f", got)
	}
	if got := similarity("DATABASE_URL", "ENABLE_METRICS"); got >= DefaultThreshold {
		t.Fatalf("unrelated similarity=%f", got)
	}
}
