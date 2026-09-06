package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClassifyExit(t *testing.T) {
	exp := ExpectedIdentity{ExitIP: "1.2.3.4", ASN: "AS3561", Country: "US"}
	if v, _ := ClassifyExit(ExitIdentity{IP: "1.2.3.4", ASN: "AS3561 CenturyLink", Country: "US"}, exp); v != ExitMatched {
		t.Fatalf("want matched")
	}
	if v, _ := ClassifyExit(ExitIdentity{IP: "9.9.9.9", ASN: "AS3561", Country: "US"}, exp); v != ExitMismatched {
		t.Fatalf("want mismatched on IP")
	}
	if v, _ := ClassifyExit(ExitIdentity{IP: "1.2.3.4", ASN: "AS3561", Country: "us"}, exp); v != ExitMatched {
		t.Fatalf("country should be case-insensitive")
	}
	if v, _ := ClassifyExit(ExitIdentity{IP: "1.2.3.4", ASN: "AS3561", Country: "US"}, ExpectedIdentity{}); v != ExitUnverifiable {
		t.Fatalf("no expected -> unverifiable")
	}
}

func TestMixedPortProbeFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ip":"203.0.113.8","org":"AS3561 CenturyLink","country":"US"}`))
	}))
	defer srv.Close()
	p := MixedPortProbe{URL: srv.URL, Client: srv.Client()}
	id, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if id.IP != "203.0.113.8" || id.ASN != "AS3561 CenturyLink" || id.Country != "US" {
		t.Fatalf("id = %+v", id)
	}
}

func TestMixedPortProbeFetchNon2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"status":429,"error":{"title":"Rate limit"}}`))
	}))
	defer srv.Close()
	p := MixedPortProbe{URL: srv.URL, Client: srv.Client()}

	if _, err := p.Fetch(context.Background()); err == nil {
		t.Fatal("429 must return an error, not an empty identity")
	}
}

func TestMixedPortProbeFetchEmptyIPIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	p := MixedPortProbe{URL: srv.URL, Client: srv.Client()}

	if _, err := p.Fetch(context.Background()); err == nil {
		t.Fatal("200 with empty body must return an error, not an empty identity")
	}
}
