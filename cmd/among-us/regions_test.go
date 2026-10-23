package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"openpak/among-us/internal/routing"
)

// fakeImpostor answers like home's Impostor: PUT is its own address, a join
// is whatever it holds for the code.
func fakeImpostor(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/games":
			io.WriteString(w, `{"Ip":16777343,"Port":22603}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/games":
			io.WriteString(w, `{"Ip":16777343,"Port":22603,"for":"`+r.URL.Query().Get("gameId")+`"}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/games/"):
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"Errors":[{"Reason":3}],"Game":null}`)
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakeCore serves a routing table with these nodes.
func fakeCore(t *testing.T, nodes []routing.Node) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routing.PathTable || r.Header.Get(routing.KeyHeader) != "k" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		json.NewEncoder(w).Encode(routing.NewTable(nodes))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func routerFor(t *testing.T, nodes []routing.Node) *router {
	c := &routing.Client{BaseURL: fakeCore(t, nodes).URL, Key: "k"}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	return newRouter(c, "rk")
}

type reply struct {
	status int
	ctype  string
	body   string
}

func do(t *testing.T, base, method, host, path, body string, hdr map[string]string) reply {
	t.Helper()
	req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
	req.Host = host
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return reply{res.StatusCode, res.Header.Get("Content-Type"), string(b)}
}

var calls = []struct{ method, host, path string }{
	{"PUT", "matchmaker.among.us", "/api/games"},
	{"PUT", "matchmaker-eu.among.us", "/api/games"},
	{"PUT", "matchmaker-as.among.us", "/api/games"},
	{"PUT", "na.mm.among.us:443", "/api/games"},
	{"PUT", "unknown.example", "/api/games"},
	{"GET", "matchmaker.among.us", "/api/games/123456"},
	{"POST", "matchmaker.among.us", "/api/games?gameId=123456"},
	{"POST", "matchmaker.among.us", PathRooms},
}

// Federation off (no OPENPAK_ROUTING_URL) and federation on with no nodes must
// both give exactly what home's Impostor gives today, for create and join.
func TestHomeOnlyIsUnchanged(t *testing.T) {
	imp := fakeImpostor(t)
	u, _ := url.Parse(imp.URL)

	t.Setenv("OPENPAK_ROUTING_URL", "")
	if routerFromEnv() != nil {
		t.Fatal("routing on without OPENPAK_ROUTING_URL")
	}

	// A room reported for a node that is not in the table is home's too.
	empty := routerFor(t, nil)
	empty.rooms[123456] = room{node: "gone-1"}
	// Nodes that cannot serve: no impostor role, a test node, a hostname.
	noUse := routerFor(t, []routing.Node{
		{ID: "us-photon", Region: "us-east", Services: map[string]string{"photon": "198.51.100.1:5055"}},
		{ID: "us-test", Region: "us-east", Test: true, Services: map[string]string{"impostor": "198.51.100.2:22603"}},
		{ID: "as-host", Region: "ap-southeast", Services: map[string]string{"impostor": "node.example:22603"}},
	})

	fronts := map[string]*httptest.Server{
		"off":         httptest.NewServer(front(newProxy(u), nil)),
		"empty table": httptest.NewServer(front(newProxy(u), empty)),
		"no usable":   httptest.NewServer(front(newProxy(u), noUse)),
	}
	for name, f := range fronts {
		defer f.Close()
		for _, c := range calls {
			want := do(t, imp.URL, c.method, c.host, c.path, "{}", nil)
			got := do(t, f.URL, c.method, c.host, c.path, "{}", nil)
			if c.path == PathRooms && name != "off" {
				continue // the report endpoint exists once a report key is set
			}
			if got != want {
				t.Errorf("%s: %s %s%s = %+v, want %+v", name, c.method, c.host, c.path, got, want)
			}
		}
	}
}

func TestCreateGoesToTheRegionNode(t *testing.T) {
	imp := fakeImpostor(t)
	u, _ := url.Parse(imp.URL)
	rt := routerFor(t, []routing.Node{
		{ID: "us-1", Region: "us-east", Services: map[string]string{"impostor": "203.0.113.7:22603"}},
	})
	f := httptest.NewServer(front(newProxy(u), rt))
	defer f.Close()

	// 203.0.113.7 with the first octet in the low byte.
	const wantNode = `{"Ip":124846283,"Port":22603}`
	for host, want := range map[string]string{
		"matchmaker.among.us":    wantNode,
		"na.mm.among.us":         wantNode,
		"matchmaker-eu.among.us": `{"Ip":16777343,"Port":22603}`, // eu is home's region
		"matchmaker-as.among.us": `{"Ip":16777343,"Port":22603}`, // no node in Asia
	} {
		if got := do(t, f.URL, "PUT", host, "/api/games", "", nil).body; got != want {
			t.Errorf("PUT via %s = %s, want %s", host, got, want)
		}
	}
}

func TestJoinGoesToTheRoomsNode(t *testing.T) {
	imp := fakeImpostor(t)
	u, _ := url.Parse(imp.URL)
	rt := routerFor(t, []routing.Node{
		{ID: "us-1", Region: "us-east", Services: map[string]string{"impostor": "203.0.113.7:22603"}},
	})
	f := httptest.NewServer(front(newProxy(u), rt))
	defer f.Close()
	key := map[string]string{RoomKeyHeader: "rk"}

	if r := do(t, f.URL, "POST", "", PathRooms, `{"node":"us-1","code":123456}`, map[string]string{RoomKeyHeader: "no"}); r.status != http.StatusUnauthorized {
		t.Fatalf("report with a wrong key = %d", r.status)
	}
	if r := do(t, f.URL, "POST", "", PathRooms, `{"node":"us-1","code":123456,"game":{"HostName":"h","Port":1,"IP":2}}`, key); r.status != http.StatusNoContent {
		t.Fatalf("report = %d", r.status)
	}

	// A joiner from any region hostname goes to the room's node.
	got := do(t, f.URL, "GET", "matchmaker-eu.among.us", "/api/games/123456", "", nil).body
	if want := `{"Errors":null,"Game":{"GameId":123456,"HostName":"h","IP":124846283,"Port":22603}}`; got != want {
		t.Errorf("join = %s, want %s", got, want)
	}
	if got := do(t, f.URL, "POST", "matchmaker-as.among.us", "/api/games?gameId=123456", "", nil).body; got != `{"Ip":124846283,"Port":22603}` {
		t.Errorf("legacy join = %s", got)
	}
	// Other codes are home's.
	if got := do(t, f.URL, "GET", "matchmaker.among.us", "/api/games/654321", "", nil); got.status != http.StatusNotFound {
		t.Errorf("unreported code = %+v", got)
	}

	do(t, f.URL, "POST", "", PathRooms, `{"node":"us-1","code":123456,"closed":true}`, key)
	if got := do(t, f.URL, "GET", "matchmaker.among.us", "/api/games/123456", "", nil); got.status != http.StatusNotFound {
		t.Errorf("closed room still answered: %+v", got)
	}
}

func TestRegionToken(t *testing.T) {
	for host, want := range map[string]string{
		"matchmaker.among.us": "na", "Matchmaker-EU.among.us.": "eu", "matchmaker-as.among.us:443": "as",
		"na.mm.among.us": "na", "eu.mm.among.us": "eu", "as.mm.among.us": "as", "among.us": "", "": "",
	} {
		if got := regionToken(host); got != want {
			t.Errorf("regionToken(%q) = %q, want %q", host, got, want)
		}
	}
}
