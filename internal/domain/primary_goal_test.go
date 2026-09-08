package domain

import "testing"

func TestPrimaryGoalValidate(t *testing.T) {
	valid := PrimaryGoal{OwnerID: "owner", Language: "de", BookID: "book"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid primary goal rejected: %v", err)
	}

	for _, goal := range []PrimaryGoal{
		{BookID: "book"},
		{OwnerID: "owner", BookID: "book"},
		{OwnerID: "owner", Language: " ", BookID: "book"},
		{OwnerID: " ", Language: "de", BookID: "book"},
		{OwnerID: "owner", Language: "de", BookID: "\t"},
	} {
		if err := goal.Validate(); err == nil {
			t.Fatalf("invalid primary goal accepted: %+v", goal)
		}
	}
}
