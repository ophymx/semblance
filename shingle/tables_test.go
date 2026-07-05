package shingle

import "testing"

// These pin the frozen tokenizer tables with literal expectations,
// independent of the toolchain's unicode package — that independence is
// the whole point of committing the tables, so this test must NOT compare
// against stdlib unicode (which would reintroduce the coupling and break
// on a future Unicode bump). A failure here means the committed data
// changed and stored signatures with these runes may have moved.

func TestFrozenUnicodeVersion(t *testing.T) {
	if UnicodeVersion != "15.0.0" {
		t.Errorf("frozen Unicode version = %q, want 15.0.0 (regenerating is a deliberate, signature-affecting change)", UnicodeVersion)
	}
}

func TestFrozenClassification(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'é', true},  // Latin-1 letter
		{'ĝ', true},  // Latin Extended letter
		{'Ω', true},  // Greek letter
		{'私', true},  // CJK
		{'٣', true},  // Arabic-Indic digit (Number)
		{'Ⅷ', true},  // Roman numeral (Number, letterlike)
		{'𝕏', true},  // astral (R32) mathematical letter
		{'—', false}, // em dash (punctuation)
		{'‍', false}, // zero-width joiner
		{' ', false}, // NBSP-adjacent space handled elsewhere; plain space
		{'§', false}, // section sign (symbol)
	}
	for _, tt := range tests {
		if got := isTokenRune(tt.r); got != tt.want {
			t.Errorf("isTokenRune(%q / %U) = %v, want %v", tt.r, tt.r, got, tt.want)
		}
	}
}

func TestFrozenLowercase(t *testing.T) {
	tests := []struct{ in, want rune }{
		{'É', 'é'},
		{'Ĝ', 'ĝ'},
		{'Ω', 'ω'},
		{'Ö', 'ö'},
		{'𝕏', '𝕏'}, // no lowercase form: unchanged
		{'é', 'é'}, // already lower: unchanged
		{'私', '私'}, // no case: unchanged
	}
	for _, tt := range tests {
		if got := lowerRune(tt.in); got != tt.want {
			t.Errorf("lowerRune(%q / %U) = %q, want %q", tt.in, tt.in, got, tt.want)
		}
	}
}
