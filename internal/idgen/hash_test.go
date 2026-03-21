package idgen

import (
	"regexp"
	"testing"
)

func TestGenerateHashID_Deterministic(t *testing.T) {
	a := GenerateHashID("orc", "title", "desc", "alice", "2025-01-01T00:00:00Z", 3, 0)
	b := GenerateHashID("orc", "title", "desc", "alice", "2025-01-01T00:00:00Z", 3, 0)
	if a != b {
		t.Fatalf("expected deterministic output, got %q and %q", a, b)
	}
}

func TestGenerateHashID_NonceChangesOutput(t *testing.T) {
	a := GenerateHashID("orc", "title", "desc", "alice", "2025-01-01T00:00:00Z", 3, 0)
	b := GenerateHashID("orc", "title", "desc", "alice", "2025-01-01T00:00:00Z", 3, 1)
	if a == b {
		t.Fatalf("different nonces should produce different IDs, both got %q", a)
	}
}

func TestGenerateHashID_Format(t *testing.T) {
	for _, length := range []int{3, 4, 5, 6, 7, 8} {
		id := GenerateHashID("bd", "test", "", "", "now", length, 0)
		pattern := regexp.MustCompile(`^bd-[a-z0-9]{` + string(rune('0'+length)) + `}$`)
		if !pattern.MatchString(id) {
			t.Errorf("length %d: id %q does not match expected pattern", length, id)
		}
	}
}

func TestGenerateHashID_DifferentInputs(t *testing.T) {
	a := GenerateHashID("orc", "title-a", "", "", "now", 3, 0)
	b := GenerateHashID("orc", "title-b", "", "", "now", 3, 0)
	if a == b {
		t.Fatalf("different titles should produce different IDs, both got %q", a)
	}
}

func TestComputeAdaptiveLength(t *testing.T) {
	tests := []struct {
		count   int
		wantMin int
	}{
		{0, 3},
		{10, 3},
		{100, 3},
		{200, 4},   // ~0.4% collision prob at 3 chars, well under 25%, but let's verify
		{1000, 4},  // 36^3=46656, n=1000 → P≈1-exp(-1e6/93312)≈1.0 → needs 4
		{50000, 5}, // 36^4=1.68M, n=50000 → P≈1-exp(-2.5e9/3.36e6)≈1.0 → needs 5
	}

	for _, tt := range tests {
		got := ComputeAdaptiveLength(tt.count, 3, 8, 0.25)
		if got < tt.wantMin {
			t.Errorf("count=%d: got length %d, want at least %d", tt.count, got, tt.wantMin)
		}
	}
}

func TestComputeAdaptiveLength_SmallDB(t *testing.T) {
	// With few items, length should stay at minimum
	got := ComputeAdaptiveLength(5, 3, 8, 0.25)
	if got != 3 {
		t.Errorf("5 items: got length %d, want 3", got)
	}
}

func TestComputeAdaptiveLength_RespectsMax(t *testing.T) {
	// Even with a huge count, should not exceed max
	got := ComputeAdaptiveLength(1_000_000_000, 3, 8, 0.25)
	if got > 8 {
		t.Errorf("got length %d, want at most 8", got)
	}
}
