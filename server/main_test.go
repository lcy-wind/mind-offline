package main

import "testing"

func TestTransitions(t *testing.T) {
	for _, v := range []struct {
		from, to string
		ok       bool
	}{{"pending", "cooking", true}, {"pending", "cancelled", true}, {"cooking", "ready", true}, {"ready", "completed", true}, {"completed", "cancelled", false}, {"cancelled", "cooking", false}, {"pending", "completed", false}, {"cooking", "pending", false}, {"cancelled", "cancelled", false}} {
		if canTransition(v.from, v.to) != v.ok {
			t.Errorf("%s -> %s", v.from, v.to)
		}
	}
}
func TestDishValidation(t *testing.T) {
	d := Dish{Name: "测试", Description: "", Category: "续命主食", Emoji: "🍜", Price: 28}
	if !validDish(d) {
		t.Fatal("valid dish rejected")
	}
	d.Price = -1
	if validDish(d) {
		t.Fatal("negative price accepted")
	}
	d.Price = 28
	d.Category = "unknown"
	if validDish(d) {
		t.Fatal("unknown category accepted")
	}
}
func TestOrderValidation(t *testing.T) {
	var in orderInput
	in.RequestKey = "1234567890123456"
	in.Mood = moods[0]
	if validateOrder(in) {
		t.Fatal("empty order accepted")
	}
	in.Items = append(in.Items, struct {
		DishID   int    `json:"dish_id"`
		Quantity int    `json:"quantity"`
		Mood     string `json:"mood"`
	}{1, 1, moods[0]})
	if !validateOrder(in) {
		t.Fatal("valid order rejected")
	}
	in.Items[0].Quantity = -1
	if validateOrder(in) {
		t.Fatal("negative quantity accepted")
	}
	in.Items[0].Quantity = 11
	if validateOrder(in) {
		t.Fatal("oversized quantity accepted")
	}
}
