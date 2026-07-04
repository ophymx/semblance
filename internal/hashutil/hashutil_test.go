package hashutil

import "testing"

// The values below pin the frozen primitives: if any of these tests fail,
// stored signatures have been broken. Changing them is a major-version
// event, not a test fix.

func TestSplitMix64Golden(t *testing.T) {
	tests := []struct {
		seed uint64
		want []uint64
	}{
		// seed 0 matches the published reference vectors for splitmix64.
		{seed: 0, want: []uint64{0xe220a8397b1dcdaf, 0x6e789e6aa1b965f4, 0x06c45d188009454f, 0xf88bb8a8724c81ec}},
		{seed: 42, want: []uint64{0xbdd732262feb6e95, 0x28efe333b266f103, 0x47526757130f9f52, 0x581ce1ff0e4ae394}},
	}
	for _, tt := range tests {
		s := SplitMix64(tt.seed)
		for i, want := range tt.want {
			if got := s.Next(); got != want {
				t.Errorf("SplitMix64(%d) value %d = %#x, want %#x", tt.seed, i, got, want)
			}
		}
	}
}

func TestMixGolden(t *testing.T) {
	tests := []struct {
		name   string
		hashes []uint64
		want   uint64
	}{
		{name: "single", hashes: []uint64{0xDEADBEEF}, want: 0xa9a29eb60f11ecc5},
		{name: "pair", hashes: []uint64{1, 2}, want: 0xbf83bd739df138b1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acc := MixInit
			for _, h := range tt.hashes {
				acc = Mix(acc, h)
			}
			if acc != tt.want {
				t.Errorf("Mix chain = %#x, want %#x", acc, tt.want)
			}
		})
	}
}

func TestMixOrderSensitive(t *testing.T) {
	ab := Mix(Mix(MixInit, 1), 2)
	ba := Mix(Mix(MixInit, 2), 1)
	if ab == ba {
		t.Fatalf("Mix is order-insensitive: both orders give %#x", ab)
	}
}
