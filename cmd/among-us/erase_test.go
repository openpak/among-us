package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEraseNoOpEndpoint(t *testing.T) {
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
