package transcode

import "testing"

func TestPlanQualities(t *testing.T) {
	got := PlanQualities()
	want := []int{480, 720, 1080}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
