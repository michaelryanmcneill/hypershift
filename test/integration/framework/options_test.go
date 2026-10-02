package framework

import (
	"testing"
)

func TestModeValueSet(t *testing.T) {
	t.Run("When mode is teardown, it should be accepted", func(t *testing.T) {
		var mode Mode
		v := newModeValue(AllInOneMode, &mode)
		if err := v.Set(string(TeardownMode)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mode != TeardownMode {
			t.Fatalf("expected %q, got %q", TeardownMode, mode)
		}
	})

	t.Run("When mode is invalid, it should return an error", func(t *testing.T) {
		var mode Mode
		v := newModeValue(AllInOneMode, &mode)
		if err := v.Set("nope"); err == nil {
			t.Fatal("expected an error")
		}
	})
}
