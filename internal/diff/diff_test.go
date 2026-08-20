package diff

import (
	"reflect"
	"testing"

	"github.com/sig9org/update-radar/internal/model"
)

func TestComputeFirstRun(t *testing.T) {
	got := Compute(model.Site{}, nil, model.Snapshot{Suggested: []string{"1.0"}})
	if !got.FirstRun || got.Changed() {
		t.Fatalf("unexpected first-run diff: %#v", got)
	}
}

func TestComputeAddedAndRemoved(t *testing.T) {
	old := model.Snapshot{Suggested: []string{"1.0"}, Latest: []string{"1.0", "0.9"}}
	current := model.Snapshot{Suggested: []string{"1.1"}, Latest: []string{"1.1", "1.0"}}
	got := Compute(model.Site{}, &old, current)
	if !reflect.DeepEqual(got.Suggested.Added, []string{"1.1"}) ||
		!reflect.DeepEqual(got.Suggested.Removed, []string{"1.0"}) ||
		!reflect.DeepEqual(got.Latest.Added, []string{"1.1"}) ||
		!reflect.DeepEqual(got.Latest.Removed, []string{"0.9"}) {
		t.Fatalf("unexpected diff: %#v", got)
	}
}

func TestComputeTreatsDifferentProductStateAsFirstRun(t *testing.T) {
	previous := model.Snapshot{ProductName: "ASAv", Suggested: []string{"9.22.2"}}
	current := model.Snapshot{ProductName: "APIC", Suggested: []string{"6.1(5e)(M)"}}
	got := Compute(model.Site{}, &previous, current)
	if !got.FirstRun || got.Changed() {
		t.Fatalf("unexpected mismatched-product diff: %#v", got)
	}
}
