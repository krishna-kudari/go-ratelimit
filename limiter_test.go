package goratelimit

import (
	"testing"
)

func TestFormatKey_Plain(t *testing.T) {
	o := defaultOptions()
	got := o.FormatKey("user:123")
	want := "ratelimit:user:123"
	if got != want {
		t.Errorf("FormatKey plain: got %q, want %q", got, want)
	}
}

func TestFormatKey_HashTag(t *testing.T) {
	o := defaultOptions()
	o.HashTag = true
	got := o.FormatKey("user:123")
	want := "ratelimit:{user:123}"
	if got != want {
		t.Errorf("FormatKey hash-tag: got %q, want %q", got, want)
	}
}

func TestFormatKeySuffix_Plain(t *testing.T) {
	o := defaultOptions()
	got := o.FormatKeySuffix("user:123", "42")
	want := "ratelimit:user:123:42"
	if got != want {
		t.Errorf("FormatKeySuffix plain: got %q, want %q", got, want)
	}
}

func TestFormatKeySuffix_HashTag(t *testing.T) {
	o := defaultOptions()
	o.HashTag = true
	got := o.FormatKeySuffix("user:123", "42")
	want := "ratelimit:{user:123}:42"
	if got != want {
		t.Errorf("FormatKeySuffix hash-tag: got %q, want %q", got, want)
	}
}

func TestFormatKeySuffix_HashTag_SlotConsistency(t *testing.T) {
	o := defaultOptions()
	o.HashTag = true

	k1 := o.FormatKeySuffix("user:123", "100")
	k2 := o.FormatKeySuffix("user:123", "101")

	tag1 := extractHashTag(k1)
	tag2 := extractHashTag(k2)
	if tag1 != tag2 {
		t.Errorf("hash tags differ: %q vs %q (keys: %q, %q)", tag1, tag2, k1, k2)
	}
	if tag1 != "user:123" {
		t.Errorf("expected hash tag %q, got %q", "user:123", tag1)
	}
}

func TestWithHashTag_Option(t *testing.T) {
	o := applyOptions([]Option{WithHashTag()})
	if !o.HashTag {
		t.Error("WithHashTag should set HashTag to true")
	}
	got := o.FormatKey("ip:10.0.0.1")
	want := "ratelimit:{ip:10.0.0.1}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatKey_CustomPrefix_HashTag(t *testing.T) {
	o := applyOptions([]Option{WithKeyPrefix("myapp"), WithHashTag()})
	got := o.FormatKey("api-key-abc")
	want := "myapp:{api-key-abc}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// extractHashTag returns the content between the first { and the next }.
func extractHashTag(key string) string {
	start := -1
	for i, c := range key {
		if c == '{' {
			start = i + 1
		} else if c == '}' && start >= 0 {
			return key[start:i]
		}
	}
	return ""
}
