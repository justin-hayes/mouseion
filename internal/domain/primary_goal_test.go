package domain

import "testing"

func TestPrimaryGoalValidate(t *testing.T) {
	valid := PrimaryGoal{OwnerID: "owner", BookID: "book"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid primary goal rejected: %v", err)
	}

	for _, goal := range []PrimaryGoal{
		{BookID: "book"},
		{OwnerID: "owner"},
		{OwnerID: " ", BookID: "book"},
		{OwnerID: "owner", BookID: "\t"},
	} {
		if err := goal.Validate(); err == nil {
			t.Fatalf("invalid primary goal accepted: %+v", goal)
		}
	}
}
