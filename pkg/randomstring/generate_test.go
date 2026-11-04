package randomstring

import (
	"testing"
)

func TestGenerate(t *testing.T) {
	s1 := Generate(16)
	s2 := Generate(16)
	if len(s1) != 16 {
		t.Fatalf("expected length 16, got %d", len(s1))
	}
	if s1 == s2 {
		t.Fatalf("expected two random strings to differ, got %q", s1)
	}
	for _, r := range s1 {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z')) {
			t.Errorf("unexpected character %q in base-36 string %q", r, s1)
		}
	}
}
