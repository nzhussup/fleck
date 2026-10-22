package catalog

import (
	"net/http"
)

func Products(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"products":[]}`))
}
