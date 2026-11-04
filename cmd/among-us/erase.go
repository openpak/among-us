package main

// Account erasure (openpak/prds#120): DELETE /internal/accounts/{account_id}/data with
// `Authorization: Bearer <ACCOUNT_ERASE_KEY>`. The front holds no per-account data (see docs/deploy.md): its only state is a room code -> region node table and an unused routing cache.
// The endpoint is a logged no-op that answers rows=0, so the website finaliser has a step to call
// and a missing service stays visible. Unset ACCOUNT_ERASE_KEY: 503 (never open).

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func eraseHandler(service string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := os.Getenv("ACCOUNT_ERASE_KEY")
		reply := func(code int, v any) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(v)
		}
		if key == "" {
			reply(http.StatusServiceUnavailable, map[string]any{"error": "erase not configured"})
			return
		}
		auth := r.Header.Get("Authorization")
		got := strings.TrimPrefix(auth, "Bearer ")
		if got == auth || subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
			reply(http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		id := r.PathValue("account_id")
		if !uuidRe.MatchString(id) {
			reply(http.StatusBadRequest, map[string]any{"error": "account_id must be a UUID"})
			return
		}
		log.Printf("account erased account=%s rows=%d service=%s", strings.ToLower(id), 0, service)
		reply(http.StatusOK, map[string]any{"service": service, "rows": 0})
	}
}
