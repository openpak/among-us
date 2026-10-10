package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.b.String()
}

// The matchmaker front sees tokens, game codes and client addresses; its log must carry none of
// them, whether the request proxies through (first front) or the upstream is gone (second).
func TestLogsCarryNoPersonalData(t *testing.T) {
	var out syncBuf
	prev := log.Writer()
	log.SetOutput(&out)
	defer log.SetOutput(prev)

	const (
		code   = "QWERTZ9"
		secret = "mm-token-s3cr3t"
		query  = "q-token-9f8e7d"
		name   = "Zorblaxa"
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "{}") }))
	defer upstream.Close()
	uURL, _ := url.Parse(upstream.URL)
	live := httptest.NewServer(front(newProxy(uURL)))
	defer live.Close()

	// A second front whose upstream is dead, so the proxy error path logs.
	deadSrv := httptest.NewServer(http.NotFoundHandler())
	deadURL, _ := url.Parse(deadSrv.URL)
	deadSrv.Close()
	dead := httptest.NewServer(front(newProxy(deadURL)))
	defer dead.Close()

	for _, base := range []string{live.URL, dead.URL} {
		for _, p := range []string{
			"/api/games/" + code + "?token=" + query,
			"/api/user",
			"/nintendo/profile/" + name + "/12345",
		} {
			req, _ := http.NewRequest("POST", base+p, strings.NewReader(`{"name":"`+name+`"}`))
			req.Header.Set("Authorization", "Bearer "+secret)
			req.Header.Set("Cookie", "sid=cookie-secret")
			if res, err := http.DefaultClient.Do(req); err == nil {
				res.Body.Close()
			}
		}
	}

	got := out.String()
	if !strings.Contains(got, "known POST /api/games/~") || !strings.Contains(got, "NEW POST /nintendo/profile/~") {
		t.Fatalf("route lines missing or untagged:\n%s", got)
	}
	for _, s := range []string{code, secret, query, name, "12345", "cookie-secret", "127.0.0.1", upstream.Listener.Addr().String()} {
		if strings.Contains(got, s) {
			t.Errorf("log contains %q:\n%s", s, got)
		}
	}
}
