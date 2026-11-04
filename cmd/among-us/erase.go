package main

// Account erasure (openpak/prds#120): DELETE /internal/accounts/{account_id}/data with
// `Authorization: Bearer <ACCOUNT_ERASE_KEY>`. The front holds no per-account data (see docs/deploy.md): its only state is a room code -> region node table and an unused routing cache.
// The player data lives in Impostor, which listens on loopback only, so this endpoint is the
// container's one door: it forwards the same call to Impostor (IMPOSTOR_ERASE_URL, default
// http://127.0.0.1:22601) and answers with Impostor's row count. Unset ACCOUNT_ERASE_KEY: 503 (never open).

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
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
		rows, err := eraseImpostor(r, id, key)
		if err != nil {
			log.Printf("account erase failed account=%s service=%s: %v", strings.ToLower(id), service, err)
			reply(http.StatusBadGateway, map[string]any{"error": "impostor erase failed"})
			return
		}
		log.Printf("account erased account=%s rows=%d service=%s", strings.ToLower(id), rows, service)
		reply(http.StatusOK, map[string]any{"service": service, "rows": rows})
	}
}

// eraseImpostor forwards the erase to Impostor on loopback and returns its row count.
func eraseImpostor(r *http.Request, id, key string) (int, error) {
	base := os.Getenv("IMPOSTOR_ERASE_URL")
	if base == "" {
		base = "http://127.0.0.1:22601"
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodDelete, strings.TrimRight(base, "/")+"/internal/accounts/"+id+"/data", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("impostor answered %s", resp.Status)
	}
	var out struct {
		Rows int `json:"rows"`
	}
	_ = json.Unmarshal(body, &out)
	return out.Rows, nil
}
