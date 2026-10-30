package main

import "testing"

type seedRow struct {
	Name string
	Alt  string
}

func TestPlanCatalog(t *testing.T) {
	existing := []*seedRow{
		{Name: "Alpha", Alt: "A"},
		{Name: "Beta", Alt: "B"},
	}
	nameOf := func(r *seedRow) string { return r.Name }
	altOf := func(r *seedRow) string { return r.Alt }
	unchanged := func(*seedRow) bool { return false }
	changed := func(*seedRow) bool { return true }

	tests := []struct {
		name     string
		items    []*seedRow
		lookup   string
		alt      string
		changed  func(*seedRow) bool
		wantItem *seedRow
		wantPlan catalogPlan
	}{
		{
			name:     "new item",
			items:    existing,
			lookup:   "Gamma",
			alt:      "G",
			changed:  changed,
			wantPlan: planCreate,
		},
		{
			name:     "existing unchanged",
			items:    existing,
			lookup:   "Alpha",
			alt:      "A",
			changed:  unchanged,
			wantItem: existing[0],
			wantPlan: planSkip,
		},
		{
			name:     "existing with changed mutable field",
			items:    existing,
			lookup:   "Alpha",
			alt:      "A",
			changed:  changed,
			wantItem: existing[0],
			wantPlan: planUpdate,
		},
		{
			name:     "dual key matches by secondary key",
			items:    existing,
			lookup:   "Renamed",
			alt:      "B",
			changed:  changed,
			wantItem: existing[1],
			wantPlan: planUpdate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotItem, gotPlan := planCatalog(tt.items, tt.lookup, tt.alt, nameOf, altOf, tt.changed)
			if gotPlan != tt.wantPlan {
				t.Fatalf("plan = %v, want %v", gotPlan, tt.wantPlan)
			}
			if gotItem != tt.wantItem {
				t.Fatalf("item = %v, want %v", gotItem, tt.wantItem)
			}
		})
	}
}
