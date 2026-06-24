package bias

import "testing"

func TestDetect_EmptyEvents(t *testing.T) {
	results := Detect([]Event{})
	if len(results) != 0 {
		t.Errorf("expected 0 detections, got %d", len(results))
	}
}

func TestDetect_NoBias(t *testing.T) {
	events := []Event{
		{EventType: "symptom_mentioned", SequenceNumber: 1},
		{EventType: "differential_explored", SequenceNumber: 2},
		{EventType: "differential_explored", SequenceNumber: 3},
		{EventType: "question_asked", SequenceNumber: 4},
		{EventType: "hypothesis_proposed", SequenceNumber: 5},
		{EventType: "hypothesis_committed", SequenceNumber: 6},
		{EventType: "new_info_received", SequenceNumber: 7},
		{EventType: "hypothesis_revised", SequenceNumber: 8},
	}
	results := Detect(events)
	if len(results) != 0 {
		t.Errorf("expected 0 detections, got %d: %+v", len(results), results)
	}
}

func TestDetect_PrematureClosure(t *testing.T) {
	// hypothesis committed in sequence 2, differential explored=0 
	events := []Event{
		{EventType: "symptom_mentioned", SequenceNumber: 1},
		{EventType: "hypothesis_committed", SequenceNumber: 2},
	}
	results := Detect(events)
	if len(results) != 1 {
		t.Fatalf("expected 1 detection, got %d", len(results))
	}
	if results[0].BiasType != "premature_closure" {
		t.Errorf("expected premature_closure, got %s", results[0].BiasType)
	}
	if results[0].DetectedAtSequence != 2 {
		t.Errorf("expected detected at sequence 2, got %d", results[0].DetectedAtSequence)
	}
}

func TestDetect_PrematureClosure_NotTriggeredWhenLate(t *testing.T) {
	// hypothesis committed in sequence 6 (>= threshold 5) so no trigger
	events := []Event{
		{EventType: "symptom_mentioned", SequenceNumber: 1},
		{EventType: "differential_explored", SequenceNumber: 2},
		{EventType: "differential_explored", SequenceNumber: 3},
		{EventType: "question_asked", SequenceNumber: 4},
		{EventType: "hypothesis_proposed", SequenceNumber: 5},
		{EventType: "hypothesis_committed", SequenceNumber: 6},
	}
	results := Detect(events)
	for _, r := range results {
		if r.BiasType == "premature_closure" {
			t.Errorf("premature_closure should not trigger when committed at sequence >= %d", prematureClosureMaxSequence)
		}
	}
}

func TestDetect_AnchoringBias(t *testing.T) {
	// new info received in sequence 7, no hypothesis revised after it
	events := []Event{
		{EventType: "symptom_mentioned", SequenceNumber: 1},
		{EventType: "differential_explored", SequenceNumber: 2},
		{EventType: "differential_explored", SequenceNumber: 3},
		{EventType: "hypothesis_committed", SequenceNumber: 6},
		{EventType: "new_info_received", SequenceNumber: 7},
	}
	results := Detect(events)
	found := false
	for _, r := range results {
		if r.BiasType == "anchoring_bias" {
			found = true
			if r.DetectedAtSequence != 7 {
				t.Errorf("expected detected at sequence 7, got %d", r.DetectedAtSequence)
			}
		}
	}
	if !found {
		t.Error("expected anchoring_bias detection, got none")
	}
}

func TestDetect_AnchoringBias_NotTriggeredWhenRevised(t *testing.T) {
	//new info received followed by hypothesis revised (no trigger)
	events := []Event{
		{EventType: "new_info_received", SequenceNumber: 5},
		{EventType: "hypothesis_revised", SequenceNumber: 6},
	}
	results := Detect(events)
	for _, r := range results {
		if r.BiasType == "anchoring_bias" {
			t.Error("anchoring_bias should not trigger when hypothesis is revised after new info")
		}
	}
}
