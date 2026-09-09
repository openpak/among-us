package main

import "testing"

func TestClassify(t *testing.T) {
	cases := map[string]string{
		"/":                   "known",
		"/api/user":           "known",
		"/api/games":          "known",
		"/api/games/12345":    "known",
		"/api/games/filtered": "known",
		"/api/userdata":       "NEW", // prefix of a known route, but not one
		"/healthz2":           "NEW",
		"/nintendo/whatever":  "NEW",
	}
	for path, want := range cases {
		if got := classify(path); got != want {
			t.Errorf("classify(%q) = %q, want %q", path, got, want)
		}
	}
}
