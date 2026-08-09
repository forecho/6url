package publictoken

import "testing"

func TestAudienceSeparation(t *testing.T) {
	raw, err := Generate(Submission)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidShape(raw, Submission) || ValidShape(raw, AdApplication) {
		t.Fatal("token shape did not enforce the audience")
	}
	pepper := "01234567890123456789012345678901"
	if Hash(raw, pepper, Submission) == Hash(raw, pepper, AdApplication) {
		t.Fatal("audiences produced the same digest")
	}
}
