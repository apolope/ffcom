package httpapi

import "net/http"

// GET /healthz — sem autenticação, só para healthcheck de infraestrutura
// (Docker, load balancer). Não expõe estado de conexão com o Postgres.
func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
