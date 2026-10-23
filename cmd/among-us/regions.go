package main

// Region routing (federation-worldwide-plan.md, "Among Us (Impostor)").
//
// Every region hostname reaches this one home matchmaker, so a room code made
// under one region is found from any other. Only the game-server address in
// two replies changes:
//
//   - PUT /api/games (host a new game): the hostname the host connected to is
//     their region choice; routing.Table.Resolve picks a node, and the reply
//     names that node's Impostor ("impostor" role, "ip:port"). Home, an
//     unknown name or an unusable address proxies to home's Impostor as before.
//   - GET /api/games/{code} and the legacy POST /api/games?gameId= (join by
//     code): a code a node reported is answered with that node's address.
//     Every other code proxies to home's Impostor as before.
//
// The game code does not exist yet when the host asks where to host (the
// node's Impostor makes it over UDP afterwards), so the node reports it to
// POST /internal/v1/amongus/rooms. Reports live in memory for roomTTL.

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"openpak/among-us/internal/routing"
	"openpak/among-us/redact"
)

// PathRooms is where a node reports the rooms it hosts.
const PathRooms = "/internal/v1/amongus/rooms"

// RoomKeyHeader carries AMONG_US_ROOM_REPORT_KEY on a room report.
const RoomKeyHeader = "X-Room-Key"

// roomTTL is how long a report lasts; a node re-reports a live room before then.
const roomTTL = 6 * time.Hour

// regionToken maps the matchmaker hostname a client connected to onto the
// "amongus" region token. Anything else is no choice.
func regionToken(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	switch strings.TrimSuffix(strings.ToLower(host), ".") {
	case "matchmaker.among.us", "na.mm.among.us":
		return "na"
	case "matchmaker-eu.among.us", "eu.mm.among.us":
		return "eu"
	case "matchmaker-as.among.us", "as.mm.among.us":
		return "as"
	}
	return ""
}

// amongUsIP encodes an IPv4 address the way Impostor's replies do
// (IPAddress.Address: the first octet in the low byte).
func amongUsIP(a netip.Addr) uint32 {
	b := a.As4()
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// nodeAddr parses a node's "impostor" service ("ip:port", IPv4 only: the
// client takes a 32-bit address).
func nodeAddr(s string) (uint32, uint16, bool) {
	ap, err := netip.ParseAddrPort(s)
	if err != nil || !ap.Addr().Unmap().Is4() || ap.Port() == 0 {
		return 0, 0, false
	}
	return amongUsIP(ap.Addr().Unmap()), ap.Port(), true
}

type room struct {
	node string
	game json.RawMessage
	at   time.Time
}

// router holds the routing table client and the room codes nodes reported.
type router struct {
	client    *routing.Client
	reportKey string // "" = no room reports accepted

	mu    sync.Mutex
	rooms map[int]room
}

func newRouter(c *routing.Client, reportKey string) *router {
	return &router{client: c, reportKey: reportKey, rooms: map[int]room{}}
}

// The matchmaker cannot name a Switch account (Impostor's token carries the
// game's own product user id, not a BAAS id), so placement goes by the region
// token alone: no country, not a tester.
func (rt *router) place(token string) routing.Result {
	return rt.client.Resolve(routing.Request{Stack: routing.StackAmongUs, Token: token, Service: "impostor"})
}

// create answers PUT /api/games for a node, or reports false for home.
func (rt *router) create(w http.ResponseWriter, r *http.Request) bool {
	res := rt.place(regionToken(r.Host))
	if res.IsHome() {
		return false
	}
	ip, port, ok := nodeAddr(res.Address)
	if !ok {
		log.Printf("create: node %s has no usable impostor address, home", res.Node)
		return false
	}
	log.Printf("create: region %s node %s via %s", res.Region, res.Node, res.Via)
	writeJSON(w, map[string]any{"Ip": int64(ip), "Port": port})
	return true
}

// lookup is a reported room's node address, if the room and the node are both live.
func (rt *router) lookup(code int) (uint32, uint16, room, bool) {
	rt.mu.Lock()
	e, ok := rt.rooms[code]
	if ok && time.Since(e.at) > roomTTL {
		delete(rt.rooms, code)
		ok = false
	}
	rt.mu.Unlock()
	if !ok {
		return 0, 0, room{}, false
	}
	n, ok := rt.client.Table().Node(e.node)
	if !ok {
		return 0, 0, room{}, false
	}
	ip, port, ok := nodeAddr(n.Services["impostor"])
	return ip, port, e, ok
}

// join answers a join by code for a room a node reported, or reports false.
func (rt *router) join(w http.ResponseWriter, r *http.Request, legacy bool) bool {
	var s string
	if legacy {
		s = r.URL.Query().Get("gameId")
	} else {
		s = strings.TrimPrefix(r.URL.Path, "/api/games/")
	}
	code, err := strconv.Atoi(s)
	if err != nil {
		return false
	}
	ip, port, e, ok := rt.lookup(code)
	if !ok {
		return false
	}
	log.Printf("join: room %s on node %s", redact.Tag(code), e.node)
	if legacy {
		writeJSON(w, map[string]any{"Ip": int64(ip), "Port": port})
		return true
	}
	game := map[string]any{}
	if len(e.game) > 0 {
		_ = json.Unmarshal(e.game, &game)
	}
	game["IP"], game["Port"], game["GameId"] = ip, port, code
	writeJSON(w, map[string]any{"Errors": nil, "Game": game})
	return true
}

// roomReport is what a node posts: its id, the code, and optionally the
// listing its own Impostor serves for GET /api/games/{code} ("Game"), which a
// join answer passes on with the address replaced. Closed drops the room.
type roomReport struct {
	Node   string          `json:"node"`
	Code   int             `json:"code"`
	Game   json.RawMessage `json:"game,omitempty"`
	Closed bool            `json:"closed,omitempty"`
}

func (rt *router) report(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get(RoomKeyHeader) != rt.reportKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var rep roomReport
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&rep); err != nil || rep.Node == "" || rep.Code == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	rt.mu.Lock()
	if rep.Closed {
		if rt.rooms[rep.Code].node == rep.Node {
			delete(rt.rooms, rep.Code)
		}
	} else {
		if len(rt.rooms) > 100000 {
			rt.rooms = map[int]room{}
		}
		rt.rooms[rep.Code] = room{node: rep.Node, game: rep.Game, at: time.Now()}
	}
	rt.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	b, _ := json.Marshal(v)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(b)
}

// routerFromEnv is nil (federation off: the front proxies everything, as
// before) unless OPENPAK_ROUTING_URL is set.
func routerFromEnv() *router {
	base := strings.TrimSpace(env("OPENPAK_ROUTING_URL", ""))
	if base == "" {
		return nil
	}
	c := &routing.Client{BaseURL: base, Key: env("OPENPAK_ROUTING_KEY", "")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Refresh(ctx); err != nil {
		log.Printf("routing: first table fetch failed (home until it succeeds): %s", redact.Err(err))
	}

	return newRouter(c, env("AMONG_US_ROOM_REPORT_KEY", ""))
}
