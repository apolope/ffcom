package httpapi

import (
	"encoding/json"
	"net/http"
)

// GET /healthz — sem autenticação, só para healthcheck de infraestrutura
// (Docker, load balancer). Não expõe estado de conexão com o Postgres.
// version vem de main.go (ldflags -X, ver Dockerfile), "dev" fora de um
// build versionado.
func handleHealthz(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": version,
		})
	}
}
