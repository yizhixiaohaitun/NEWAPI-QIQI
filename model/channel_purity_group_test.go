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
func TestValidateChannelPurityGroupInterval(t *testing.T) {
	g := &ChannelPurityGroup{Name: "x", IntervalMinutes: 11, Members: []ChannelPurityMember{{ChannelID: 1, IsBaseline: true}, {ChannelID: 2}}}
	if ValidateChannelPurityGroup(g) == nil {
		t.Fatal("expected interval rejection")
	}
}
