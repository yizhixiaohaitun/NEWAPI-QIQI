package channel_purity

import (
	"github.com/QuantumNous/new-api/model"
	"testing"
)

func TestAdvanceDebounceAndRecovery(t *testing.T) {
	p := DefaultAggregatePolicy()
	a := model.ChannelPurityAssessment{State: model.ChannelPurityStateHealthy}
	w := WindowResult{BaselineAvailable: true, BaselineSamples: 5, TargetSamples: 5, Similarity: .4, Confidence: .8}
	for i := 0; i < 2; i++ {
		a = Advance(a, w, p)
		if a.State != model.ChannelPurityStateSuspect {
			t.Fatalf("window %d: %s", i, a.State)
		}
	}
	a = Advance(a, w, p)
	if a.State != model.ChannelPurityStateAlert {
		t.Fatalf("expected alert, got %s", a.State)
	}
	w.Similarity = .95
	a = Advance(a, w, p)
	if a.State != model.ChannelPurityStateAlert {
		t.Fatalf("recovered too early: %s", a.State)
	}
	a = Advance(a, w, p)
	if a.State != model.ChannelPurityStateHealthy {
		t.Fatalf("expected healthy, got %s", a.State)
	}
}
func TestAdvanceBoundaryStates(t *testing.T) {
	p := DefaultAggregatePolicy()
	tests := []struct {
		w    WindowResult
		want string
	}{{WindowResult{}, model.ChannelPurityStateBaselineUnavailable}, {WindowResult{DetectorError: true}, model.ChannelPurityStateDetectorError}, {WindowResult{BaselineAvailable: true, BaselineSamples: 2, TargetSamples: 3}, model.ChannelPurityStateLowSample}}
	for _, tt := range tests {
		if got := Advance(model.ChannelPurityAssessment{}, tt.w, p).State; got != tt.want {
			t.Errorf("got %s want %s", got, tt.want)
		}
	}
}
func TestCompareSamples(t *testing.T) {
	b := []model.ChannelPuritySample{{Valid: true, StructureSignature: "choices+usage", TotalTokens: 100}, {Valid: true, StructureSignature: "choices+usage", TotalTokens: 110}}
	x := []model.ChannelPuritySample{{Valid: true, StructureSignature: "output", TotalTokens: 300}, {Valid: true, StructureSignature: "output", TotalTokens: 310}}
	s, tok, conf, e := CompareSamples(b, x)
	if s != 0 || tok >= .7 || conf <= 0 || len(e) != 2 {
		t.Fatalf("unexpected metrics %.2f %.2f %.2f %#v", s, tok, conf, e)
	}
}
