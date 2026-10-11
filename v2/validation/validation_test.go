package validation_test

import (
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/validation"
)

type form struct {
	Status  string  `json:"status" validate:"enum=active|suspended,default=active"`
	Age     int     `json:"age" validate:"min=0,max=150"`
	Name    string  `json:"name" validate:"minLen=1,maxLen=64"`
	Score   float64 `json:"score"`
	Note    *string `json:"note" validate:"maxLen=4"`
	NoRules string  `json:"no_rules"`
}

// TestParseTag covers the tag grammar and the refusals that keep a broken
// tag from being silently ignored.
func TestParseTag(t *testing.T) {
	cons, err := validation.ParseTag("enum=active|suspended,default=active,min=0,max=150,minLen=1,maxLen=64")
	if err != nil {
		t.Fatalf("ParseTag: %v", err)
	}
	if len(cons.Enum) != 2 || cons.Enum[0] != "active" {
		t.Errorf("enum = %v", cons.Enum)
	}
	if cons.Min == nil || *cons.Min != 0 || cons.Max == nil || *cons.Max != 150 {
		t.Errorf("min/max = %v/%v", cons.Min, cons.Max)
	}
	if !cons.HasDefault || cons.Default != "active" {
		t.Errorf("default = %q %v", cons.Default, cons.HasDefault)
	}

	for _, broken := range []string{
		"nonsense", "enum=", "min=abc", "minLen=-1", "min=5,max=1", "minLen=8,maxLen=2",
	} {
		if _, err := validation.ParseTag(broken); err == nil {
			t.Errorf("ParseTag(%q) = nil error, want a refusal", broken)
		}
	}

	if empty, err := validation.ParseTag(""); err != nil || !empty.Empty() {
		t.Errorf("empty tag = %v, %v; want empty constraints, nil error", empty, err)
	}
}

// TestValidateEnum: an unlisted value fails with a code naming the field and
// the rule.
func TestValidateEnum(t *testing.T) {
	err := validation.Validate(form{Status: "deleted", Name: "a"})
	if err == nil {
		t.Fatal("enum violation passed")
	}
	appErr, ok := err.(*apperror.Error)
	if !ok {
		t.Fatalf("violation is not an *apperror.Error: %v", err)
	}
	if appErr.ErrorKind() != apperror.KindInvalidArgument {
		t.Errorf("kind = %v, want invalid_argument", appErr.ErrorKind())
	}
	if appErr.ErrorCode() != "validation.status.enum" {
		t.Errorf("code = %q, want validation.status.enum", appErr.ErrorCode())
	}
	if !strings.Contains(appErr.PublicMessage(), "active, suspended") {
		t.Errorf("message = %q, want the allowed values", appErr.PublicMessage())
	}

	if err := validation.Validate(form{Status: "active", Name: "a"}); err != nil {
		t.Errorf("listed value failed: %v", err)
	}
}

// TestValidateRange: min and max bound numeric fields.
func TestValidateRange(t *testing.T) {
	if err := validation.Validate(form{Status: "active", Name: "a", Age: 200}); err == nil {
		t.Error("max violation passed")
	}
	if err := validation.Validate(form{Status: "active", Name: "a", Age: -1}); err == nil {
		t.Error("min violation passed")
	}
	if err := validation.Validate(form{Status: "active", Name: "a", Age: 42, Score: 3.5}); err != nil {
		t.Errorf("in-range values failed: %v", err)
	}
}

// TestValidateLength: minLen and maxLen bound strings, and nil pointers are
// absent rather than violating.
func TestValidateLength(t *testing.T) {
	if err := validation.Validate(form{Status: "active", Name: ""}); err == nil {
		t.Error("minLen violation passed")
	}
	long := strings.Repeat("x", 65)
	if err := validation.Validate(form{Status: "active", Name: "a", Note: &long}); err == nil {
		t.Error("maxLen violation passed")
	}
	absent := "ok"
	if err := validation.Validate(form{Status: "active", Name: "a", Note: &absent}); err != nil {
		t.Errorf("absent-in-range pointer failed: %v", err)
	}
}

// TestValidateSkipsAbsentPointer: a nil pointer is an absent field; its
// constraints do not run, because required-ness is the contract's business.
func TestValidateSkipsAbsentPointer(t *testing.T) {
	if err := validation.Validate(form{Status: "active", Name: "a"}); err != nil {
		t.Errorf("nil pointer failed validation: %v", err)
	}
}

// TestValidateIgnoresDefault: default is a form-filling hint carried by the
// contract. It creates no rule of its own — a field with only a default passes
// with any value, including the zero value — and it does not exempt a field
// from its other rules: an empty string that is not a listed enum value still
// fails, and the form's job is to send the pre-filled default.
func TestValidateIgnoresDefault(t *testing.T) {
	type hintOnly struct {
		Mode string `json:"mode" validate:"default=fast"`
	}
	if err := validation.Validate(hintOnly{}); err != nil {
		t.Errorf("a default alone created a rule: %v", err)
	}
	if err := validation.Validate(hintOnly{Mode: "slow"}); err != nil {
		t.Errorf("a value differing from the default failed: %v", err)
	}
	if err := validation.Validate(form{Name: "a"}); err == nil {
		t.Error("an empty enum value passed; the default does not exempt the field")
	}
}

// TestValidateNonStruct: a map or slice carries no field rules.
func TestValidateNonStruct(t *testing.T) {
	if err := validation.Validate(map[string]string{"status": "deleted"}); err != nil {
		t.Errorf("non-struct value failed: %v", err)
	}
	if err := validation.Validate(nil); err != nil {
		t.Errorf("nil failed: %v", err)
	}
}

// TestTagRoundTrip: Tag renders the ruleset back into tag text, and the
// rendered tag parses back into the same constraints — the inverse of
// ParseTag, which is what the from-db contract copy relies on.
func TestTagRoundTrip(t *testing.T) {
	maxLen := 64
	min := 0.0
	max := 150.0
	minLen := 1
	cons := validation.Constraints{
		Enum:       []string{"active", "suspended"},
		Min:        &min,
		Max:        &max,
		MinLen:     &minLen,
		MaxLen:     &maxLen,
		Default:    "active",
		HasDefault: true,
	}
	tag := cons.Tag()
	want := "enum=active|suspended,min=0,max=150,minLen=1,maxLen=64,default=active"
	if tag != want {
		t.Fatalf("tag = %q, want %q", tag, want)
	}
	parsed, err := validation.ParseTag(tag)
	if err != nil {
		t.Fatalf("ParseTag(%q): %v", tag, err)
	}
	if strings.Join(parsed.Enum, ",") != strings.Join(cons.Enum, ",") {
		t.Errorf("round-trip enum = %v, want %v", parsed.Enum, cons.Enum)
	}
	if parsed.Min == nil || *parsed.Min != *cons.Min || parsed.Max == nil || *parsed.Max != *cons.Max {
		t.Errorf("round-trip min/max = %v/%v", parsed.Min, parsed.Max)
	}
	if parsed.MinLen == nil || *parsed.MinLen != 1 || parsed.MaxLen == nil || *parsed.MaxLen != 64 {
		t.Errorf("round-trip lengths = %v/%v", parsed.MinLen, parsed.MaxLen)
	}
	if !parsed.HasDefault || parsed.Default != "active" {
		t.Errorf("round-trip default = %q %v", parsed.Default, parsed.HasDefault)
	}
	emptyTag := validation.Constraints{}.Tag()
	if emptyTag != "" {
		t.Errorf("empty constraints tag = %q, want empty", emptyTag)
	}
}
