package securepayload

import (
	"encoding/base64"
	"testing"
)

func TestSealOpen(t *testing.T) {
	sealer, err := New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := sealer.Seal(map[string]string{"token": "secret"}, "notification:test")
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err := sealer.Open(encoded, "notification:test", &result); err != nil {
		t.Fatal(err)
	}
	if result["token"] != "secret" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if err := sealer.Open(encoded, "notification:other", &result); err == nil {
		t.Fatal("associated data mismatch should fail")
	}
}
