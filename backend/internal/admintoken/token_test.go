package admintoken

import "testing"

func TestGenerateParseAndHash(t *testing.T) {
	raw, prefix, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Prefix(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != prefix {
		t.Fatalf("Prefix() = %q, want %q", parsed, prefix)
	}
	hash := Hash(raw, "01234567890123456789012345678901")
	if !Equal(hash, Hash(raw, "01234567890123456789012345678901")) {
		t.Fatal("hash comparison failed")
	}
	if Equal(hash, Hash(raw+"x", "01234567890123456789012345678901")) {
		t.Fatal("changed token must not match")
	}
}

func TestValidateScopes(t *testing.T) {
	if err := ValidateScopes([]string{"catalog:read", "ads:write"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateScopes([]string{"root"}); err == nil {
		t.Fatal("unsupported scope should fail")
	}
}
