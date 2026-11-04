package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEraseForwardsToImpostor(t *testing.T) {
	var gotAuth, gotPath string
	imp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		w.Write([]byte(`{"service":"impostor","rows":3}`))
	}))
	defer imp.Close()
	t.Setenv("IMPOSTOR_ERASE_URL", imp.URL)
	t.Setenv("ACCOUNT_ERASE_KEY", "k")
	r := httptest.NewRequest("DELETE", "/internal/accounts/11111111-1111-1111-1111-111111111111/data", nil)
	r.Header.Set("Authorization", "Bearer k")
	r.SetPathValue("account_id", "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	eraseHandler("among-us")(w, r)
	if w.Code != 200 || w.Body.String() != "{\"rows\":3,\"service\":\"among-us\"}\n" {
		t.Fatalf("%d %q", w.Code, w.Body.String())
	}
	if gotAuth != "Bearer k" || gotPath != "/internal/accounts/11111111-1111-1111-1111-111111111111/data" {
		t.Fatalf("forwarded %q %q", gotAuth, gotPath)
	}
	imp.Close()
	w = httptest.NewRecorder()
	eraseHandler("among-us")(w, r)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("down impostor must fail the step, got %d", w.Code)
	}
}

func TestEraseNoOpEndpoint(t *testing.T) {
	// no Impostor to forward to in this test: a stub answers rows=0
	imp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"rows":0}`)) }))
	defer imp.Close()
	t.Setenv("IMPOSTOR_ERASE_URL", imp.URL)
	const id = "11111111-1111-1111-1111-111111111111"
	call := func(env, hdr, acct string) (int, string) {
		t.Setenv("ACCOUNT_ERASE_KEY", env)
		mux := http.NewServeMux()
		mux.HandleFunc("DELETE /internal/accounts/{account_id}/data", eraseHandler("among-us"))
		r := httptest.NewRequest("DELETE", "/internal/accounts/"+acct+"/data", nil)
		if hdr != "" {
			r.Header.Set("Authorization", hdr)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	for _, c := range []struct {
		env, hdr, id string
		want         int
	}{
		{"", "Bearer k", id, 503}, {"k", "", id, 401}, {"k", "Bearer x", id, 401},
		{"k", "Bearer k", "nope", 400}, {"k", "Bearer k", id, 200},
	} {
		if got, _ := call(c.env, c.hdr, c.id); got != c.want {
			t.Errorf("%+v: got %d", c, got)
		}
	}
	if _, body := call("k", "Bearer k", id); body != "{\"rows\":0,\"service\":\"among-us\"}\n" {
		t.Errorf("body %q", body)
	}
}
