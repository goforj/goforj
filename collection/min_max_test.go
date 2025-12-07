package collection

import (
	"testing"
)

func TestMinMax(t *testing.T) {
	nums := New([]int{5, 1, 9, 3})

	val1, ok := Min(nums)
	if !ok || val1 != 1 {
		t.Fatalf("expected min=1, ok=true got min=%v ok=%v", val1, ok)
	}

	val2, ok := Max(nums)
	if !ok || val2 != 9 {
		t.Fatalf("expected max=9, ok=true got max=%v ok=%v", val2, ok)
	}
}
