package abi

import "testing"

func TestNormalizeEmail(t *testing.T) {
	good := map[string]string{
		"Alice@Example.COM":       "alice@example.com",
		"  bob@example.com  ":     "bob@example.com",
		"a.b+tag@sub.example.org": "a.b+tag@sub.example.org",
		"user@bücher.de":          "user@xn--bcher-kva.de",
		"USER@XN--BCHER-KVA.DE":   "user@xn--bcher-kva.de",
	}
	for in, want := range good {
		got, err := NormalizeEmail(in)
		if err != nil || got != want {
			t.Errorf("NormalizeEmail(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestNormalizeEmailRejectsAnythingThatIsNotOneMailbox(t *testing.T) {
	bad := []string{
		"",
		"   ",
		"no-at-sign",
		"@example.com",
		"user@",
		"Boss <boss@example.com>",              // display name form
		"a@example.com, evil@example.com",      // two recipients in one string
		"a@example.com;evil@example.com",       // semicolon list
		"a@example.com\nBcc: evil@example.com", // header injection
		"\"quoted\"@example.com",               // quoted local part
		"a@exam ple.com",                       // space
		"a(comment)@example.com",               // comment
		"a@example.com>",                       // stray bracket
	}
	for _, in := range bad {
		if got, err := NormalizeEmail(in); err == nil {
			t.Errorf("NormalizeEmail(%q) = %q, want an error", in, got)
		}
	}
}

// A lookalike domain must never normalize to the real one.
func TestNormalizeEmailKeepsLookalikesDistinct(t *testing.T) {
	real, err := NormalizeEmail("ceo@paypal.com")
	if err != nil {
		t.Fatal(err)
	}
	for _, lookalike := range []string{"ceo@paypa1.com", "ceo@раypal.com" /* Cyrillic р, а */} {
		got, err := NormalizeEmail(lookalike)
		if err != nil {
			continue // rejected outright is fine
		}
		if got == real {
			t.Errorf("%q normalized to the real address %q", lookalike, real)
		}
	}
}

func TestNormalizeAddressesDeduplicates(t *testing.T) {
	got, err := NormalizeAddresses([]Address{
		{Name: "A", Email: "A@Example.com"},
		{Name: "dup", Email: "a@example.com"},
		{Email: "b@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Email != "a@example.com" || got[0].Name != "A" || got[1].Email != "b@example.com" {
		t.Fatalf("unexpected result %+v", got)
	}
	if _, err := NormalizeAddresses([]Address{{Email: "ok@example.com"}, {Email: "bad"}}); err == nil {
		t.Fatal("one bad address must fail the whole list")
	}
}

func TestDomainOf(t *testing.T) {
	if DomainOf("a@example.com") != "example.com" || DomainOf("nope") != "" {
		t.Fatal("DomainOf misbehaves")
	}
}
