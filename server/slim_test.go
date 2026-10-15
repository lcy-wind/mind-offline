package main

import (
	"math"
	"testing"
	"time"
)

func TestSlimCalculation(t *testing.T) {
	now := time.Date(2026, 10, 14, 16, 0, 0, 0, time.UTC)
	e := slimEntry{ID: "test_record_123456", Day: "2026-10-15", Kind: "food", Name: "米饭", Quantity: 200, Rate: 130, Calories: 9999}
	if err := prepareSlim(&e, now); err != nil || e.Calories != 260 {
		t.Fatalf("food: %+v %v", e, err)
	}
	e.Kind = "exercise"
	e.Name = "平地走路（约4.5–5公里/小时）"
	e.Quantity = 30
	e.Weight = 60
	e.Rate = 999
	if err := prepareSlim(&e, now); err != nil || e.Calories != 110.3 || e.Rate != 3.5 {
		t.Fatalf("exercise: %+v %v", e, err)
	}
	for _, bad := range []slimEntry{
		{ID: e.ID, Day: "2026-10-16", Kind: "weight", Name: "体重", Weight: 60},
		{ID: e.ID, Day: "2026-02-30", Kind: "weight", Name: "体重", Weight: 60},
		{ID: e.ID, Day: e.Day, Kind: "weight", Name: "体重", Weight: math.NaN()},
		{ID: e.ID, Day: e.Day, Kind: "food", Name: "米饭", Quantity: -20, Rate: 130},
		{ID: e.ID, Day: e.Day, Kind: "exercise", Name: "瞎猜项目", Quantity: 30, Weight: 60},
	} {
		if prepareSlim(&bad, now) == nil {
			t.Fatalf("accepted invalid: %+v", bad)
		}
	}
}
