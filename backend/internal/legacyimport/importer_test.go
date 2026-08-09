package legacyimport

import "testing"

func TestSlugifyUnicodeFallbackIsStable(t *testing.T) {
	first := slugify("券商", "category")
	second := slugify("券商", "category")
	if first != second || first == "category" {
		t.Fatalf("slugify fallback is unstable: %q %q", first, second)
	}
}
