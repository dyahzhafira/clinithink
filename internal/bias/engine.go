package bias

import "fmt"

type Event struct {
	EventType      string
	SequenceNumber int
}

type DetectionResult struct {
	BiasType           string `json:"bias_type"`
	DetectedAtSequence int    `json:"detected_at_sequence"`
	ConfidenceNote     string `json:"confidence_note"`
}

const (
	prematureClosureMaxSequence  = 5
	prematureClosureMinDiffCount = 2
)

func Detect(events []Event) []DetectionResult {
	var results []DetectionResult
	if r := checkPrematureClosure(events); r != nil {
		results = append(results, *r)
	}
	if r := checkAnchoringBias(events); r != nil {
		results = append(results, *r)
	}
	return results
}

// Premature closure (hypothesis_committed appear before sequence 5 & past differential_explored < 2)
func checkPrematureClosure(events []Event) *DetectionResult {
	for _, e := range events {
		if e.EventType != "hypothesis_committed" {
			continue
		}
		if e.SequenceNumber >= prematureClosureMaxSequence {
			continue
		}
		diffCount := 0
		for _, prev := range events {
			if prev.EventType == "differential_explored" && prev.SequenceNumber < e.SequenceNumber {
				diffCount++
			}
		}
		if diffCount < prematureClosureMinDiffCount {
			return &DetectionResult{
				BiasType:           "premature_closure",
				DetectedAtSequence: e.SequenceNumber,
				ConfidenceNote: fmt.Sprintf(
					"Hipotesis dikunci pada sequence %d dengan hanya %d diferensial dieksplorasi sebelumnya",
					e.SequenceNumber, diffCount,
				),
			}
		}
	}
	return nil
}

// Anchoring bias (new info received but not followed by hypothesis revised)
func checkAnchoringBias(events []Event) *DetectionResult {
	for _, e := range events {
		if e.EventType != "new_info_received" {
			continue
		}
		revised := false
		for _, after := range events {
			if after.EventType == "hypothesis_revised" && after.SequenceNumber > e.SequenceNumber {
				revised = true
				break
			}
		}
		if !revised {
			return &DetectionResult{
				BiasType:           "anchoring_bias",
				DetectedAtSequence: e.SequenceNumber,
				ConfidenceNote: fmt.Sprintf(
					"Informasi baru diterima pada sequence %d namun hipotesis tidak direvisi",
					e.SequenceNumber,
				),
			}
		}
	}
	return nil
}
