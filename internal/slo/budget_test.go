package slo

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestParseWindowsAndSLO(t *testing.T) {
	windows, err := ParseWindows("1h/6h/3d")
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 3 || windows[2].Duration != 72*time.Hour || windows[2].Label != "3d" {
		t.Fatalf("windows = %+v", windows)
	}
	if _, err := ParseWindows("1h"); err == nil {
		t.Fatal("expected two windows")
	}
	slo, err := ParseSLO(99.9)
	if err != nil || math.Abs(slo-0.999) > 1e-9 {
		t.Fatalf("slo = %v %v", slo, err)
	}
	slo, err = ParseSLO(0.999)
	if err != nil || math.Abs(slo-0.999) > 1e-9 {
		t.Fatalf("ratio slo = %v %v", slo, err)
	}
	if _, err := ParseSLO(100); err == nil {
		t.Fatal("expected 100 to fail")
	}
}

func TestDecidePageFreezeShip(t *testing.T) {
	slo := 0.999
	page := Assemble("checkout", slo, []Burn{
		{Window: Window{Label: "1h", Duration: time.Hour}, ErrorRatio: 0.02},
		{Window: Window{Label: "6h", Duration: 6 * time.Hour}, ErrorRatio: 0.008},
		{Window: Window{Label: "3d", Duration: 72 * time.Hour}, ErrorRatio: 0.0015},
	}, false)
	if page.Decision.Action != ActionPage {
		t.Fatalf("action = %s (%s)", page.Decision.Action, page.Decision.Why)
	}
	if page.Remaining >= 0 {
		t.Fatalf("remaining = %v", page.Remaining)
	}
	if math.Abs(page.Burns[0].BurnRate-20) > 0.01 {
		t.Fatalf("1h burn = %v", page.Burns[0].BurnRate)
	}

	freeze := Assemble("checkout", slo, []Burn{
		{Window: Window{Label: "1h", Duration: time.Hour}, ErrorRatio: 0.002},
		{Window: Window{Label: "6h", Duration: 6 * time.Hour}, ErrorRatio: 0.007},
		{Window: Window{Label: "3d", Duration: 72 * time.Hour}, ErrorRatio: 0.0005},
	}, false)
	if freeze.Decision.Action != ActionFreeze {
		t.Fatalf("freeze action = %s (%s)", freeze.Decision.Action, freeze.Decision.Why)
	}

	ship := Assemble("checkout", slo, []Burn{
		{Window: Window{Label: "1h", Duration: time.Hour}, ErrorRatio: 0.0001},
		{Window: Window{Label: "6h", Duration: 6 * time.Hour}, ErrorRatio: 0.0001},
		{Window: Window{Label: "3d", Duration: 72 * time.Hour}, ErrorRatio: 0.0001},
	}, false)
	if ship.Decision.Action != ActionShip {
		t.Fatalf("ship action = %s (%s)", ship.Decision.Action, ship.Decision.Why)
	}
	if ship.Remaining < 0.8 {
		t.Fatalf("ship remaining = %v", ship.Remaining)
	}
}

func TestDemoNarrative(t *testing.T) {
	rep := Assemble("checkout", 0.999, DemoBurns("checkout"), true)
	if rep.Decision.Action != ActionPage {
		t.Fatalf("demo action = %s", rep.Decision.Action)
	}
	blob := render(rep)
	for _, want := range []string{
		"99.90%",
		"20.0x",
		"Page and freeze",
		"http_requests_total",
		`job="checkout"`,
		"[1h]",
		"[3d]",
		"sre triage checkout",
	} {
		if !strings.Contains(blob, want) {
			t.Fatalf("missing %q\n%s", want, blob)
		}
	}
}

func render(rep Report) string {
	var b strings.Builder
	for _, f := range rep.Findings {
		b.WriteString(f.Category)
		b.WriteByte(' ')
		b.WriteString(f.Phenomenon)
		b.WriteByte('\n')
		b.WriteString(f.NextStep)
		b.WriteByte('\n')
	}
	for _, n := range rep.Notes {
		b.WriteString(n)
		b.WriteByte('\n')
	}
	return b.String()
}
