package duckduckgo

import (
	"testing"
)

// These tests are offline: they exercise the URI driver's pure string functions.
// The client's HTTP behaviour is covered in duckduckgo_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "duckduckgo" {
		t.Errorf("Scheme = %q, want duckduckgo", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "duckduckgo" {
		t.Errorf("Identity.Binary = %q, want duckduckgo", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
	}{
		{"Python programming", "answer", "Python programming"},
		{"New York City", "answer", "New York City"},
		{"Albert Einstein", "answer", "Albert Einstein"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("  ")
	if err == nil {
		t.Error("Classify(\"\") should return an error for empty input")
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("answer", "Python")
	if err != nil {
		t.Fatalf("Locate error: %v", err)
	}
	if got != "https://duckduckgo.com/?q=Python" {
		t.Errorf("Locate = %q, unexpected", got)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("page", "foo")
	if err == nil {
		t.Error("Locate with unknown type should return an error")
	}
}
