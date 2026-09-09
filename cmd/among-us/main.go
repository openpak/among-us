// Command among-us fronts an unmodified Impostor server for a Nintendo Switch
// that has been redirected to Innersloth's matchmaker hostname.
//
// It does two things. It answers /healthz, because that is how the fleet asks
// whether a service is up (../../ports.md). And it reverse-proxies everything
// else to Impostor while logging the request paths, because nobody has yet seen
// what the Switch build of Among Us asks a matchmaker for; that log is the
// evidence docs/evidence-needed.md is after.
//
// It deliberately logs paths only. The matchmaker token travels in the query
// string, in the Authorization header and in the /api/user response body, and
// none of those belong in a log file.
package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

// impostorRoutes are the matchmaker paths Impostor serves, from its HTTP
// controllers: HelloController "/", TokenController "/api/user",
// GamesController "/api/games" plus "/api/games/{gameId}" and
// "/api/games/filtered". Anything outside this set is something the console
// wants and the PC client does not, which is the interesting case.
var impostorRoutes = []string{"/", "/api/user", "/api/games"}

// classify reports whether a request path is one Impostor already handles.
// A "NEW" answer is a finding, not an error: it means the console asked for
// something no public client asks for.
func classify(path string) string {
	for _, r := range impostorRoutes {
		if path == r || (r != "/" && strings.HasPrefix(path, r+"/")) {
			return "known"
		}
	}
	return "NEW"
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	addr := env("AMONG_US_HTTP_ADDR", ":22600")
	target := env("AMONG_US_IMPOSTOR_URL", "http://127.0.0.1:22601")

	impostor, err := url.Parse(target)
	if err != nil {
		log.Fatalf("AMONG_US_IMPOSTOR_URL %q is not a URL: %v", target, err)
	}
	proxy := httputil.NewSingleHostReverseProxy(impostor)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", classify(r.URL.Path), r.Method, r.URL.Path)
		proxy.ServeHTTP(w, r)
	})

	log.Printf("among-us front on %s, proxying to %s", addr, impostor)
	// ponytail: plain ListenAndServe, no timeouts, no graceful shutdown. This
	// sits behind Traefik on a private port and is restarted by the container
	// runtime. Add a configured http.Server if it ever faces anything else.
	log.Fatal(http.ListenAndServe(addr, mux))
}
