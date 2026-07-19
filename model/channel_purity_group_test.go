package model

import "testing"

func TestValidateChannelPurityGroup(t *testing.T) {
	g := &ChannelPurityGroup{Name: "group-a", IntervalMinutes: 5, Members: []ChannelPurityMember{{ChannelID: 1, IsBaseline: true}, {ChannelID: 2}}}
	if err := ValidateChannelPurityGroup(g); err != nil {
		t.Fatal(err)
	}
	if g.Members[0].BaselineSlot == nil || g.Members[1].BaselineSlot != nil {
		t.Fatal("baseline uniqueness slots not normalized")
	}
	g.Members[1].IsBaseline = true
	if err := ValidateChannelPurityGroup(g); err == nil {
		t.Fatal("expected multiple baseline rejection")
	}
}
func TestValidateChannelPurityGroupModelComparisons(t *testing.T) {
	group := &ChannelPurityGroup{Name: "models", IntervalMinutes: 5,
		Members:          []ChannelPurityMember{{ChannelID: 1, IsBaseline: true}, {ChannelID: 2}},
		ModelComparisons: []ChannelPurityModelComparison{{BaselineModel: " base ", TargetModel: " target "}},
	}
	if err := ValidateChannelPurityGroup(group); err != nil {
		t.Fatal(err)
	}
	if group.ModelComparisons[0].BaselineModel != "base" || group.ModelComparisons[0].TargetModel != "target" {
		t.Fatal("model comparisons were not normalized")
	}
	group.ModelComparisons = append(group.ModelComparisons, group.ModelComparisons[0])
	if err := ValidateChannelPurityGroup(group); err == nil {
		t.Fatal("expected duplicate model comparison rejection")
	}
}

func TestValidateChannelPurityGroupInterval(t *testing.T) {
	g := &ChannelPurityGroup{Name: "x", IntervalMinutes: 11, Members: []ChannelPurityMember{{ChannelID: 1, IsBaseline: true}, {ChannelID: 2}}}
	if ValidateChannelPurityGroup(g) == nil {
		t.Fatal("expected interval rejection")
	}
}
