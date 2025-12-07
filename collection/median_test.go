package collection

import (
	"github.com/goforj/godump"
	"testing"
)

func TestMedian_OddAndEven(t *testing.T) {
	odd := New([]int{1, 3, 2})
	med, ok := Median(odd)
	if !ok || med != 2 {
		t.Fatalf("expected median 2, got %v ok=%v", med, ok)
	}

	even := New([]int{1, 2, 3, 4})
	med, ok = Median(even)

	godump.Dump(med)

	if !ok || med != 2.5 {
		t.Fatalf("expected median 2.5, got %v ok=%v", med, ok)
	}
}
