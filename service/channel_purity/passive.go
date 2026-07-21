package channel_purity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
)

func encodeFieldProfiles(profiles []FieldProfile) string {
	if len(profiles) == 0 {
		return ""
	}
	encoded, err := json.Marshal(uniqueSortedProfiles(profiles))
	if err != nil {
		return ""
	}
	return string(encoded)
}

// EncodeFieldProfiles serializes sanitized path/type metadata only; response values are never included.
func EncodeFieldProfiles(profiles []FieldProfile) string { return encodeFieldProfiles(profiles) }

type StructureMetadata struct {
	Protocol       string          `json:"protocol,omitempty"`
	StatusCode     int             `json:"status_code,omitempty"`
	ModelFamily    string          `json:"model_family,omitempty"`
	EventSequence  []string        `json:"event_sequence,omitempty"`
	FinishReasons  []string        `json:"finish_reasons,omitempty"`
	HeaderPresence map[string]bool `json:"header_presence,omitempty"`
	HasSignatureID bool            `json:"has_signature_id,omitempty"`
}

func encodeStructureMetadata(features AnonymousFeatures) string {
	metadata := StructureMetadata{
		Protocol: features.Protocol, StatusCode: features.StatusCode, ModelFamily: features.ModelFamily,
		EventSequence: append([]string(nil), features.EventSequence...),
		FinishReasons: uniqueSorted(features.FinishReasons), HeaderPresence: features.HeaderPresence,
		HasSignatureID: features.HasSignatureID,
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// EncodeStructureMetadata serializes protocol/event/finish/header-presence dimensions without values.
func EncodeStructureMetadata(features AnonymousFeatures) string {
	return encodeStructureMetadata(features)
}

// RecordPassiveObservation persists only anonymous structural and token features
// for every enabled purity group containing the selected real-traffic channel.
func RecordPassiveObservation(channelID int, actualModel, runKey string, features AnonymousFeatures) error {
	if channelID <= 0 || strings.TrimSpace(actualModel) == "" {
		return nil
	}
	groupIDs, err := model.ListPurityGroupIDsForChannel(channelID)
	if err != nil {
		return err
	}
	signaturePayload := struct {
		Protocol      string          `json:"protocol"`
		ModelFamily   string          `json:"model_family"`
		Fields        []string        `json:"fields"`
		FieldProfiles []FieldProfile  `json:"field_profiles"`
		Events        []string        `json:"events"`
		FinishReasons []string        `json:"finish_reasons"`
		Headers       map[string]bool `json:"headers"`
		Signature     bool            `json:"signature"`
	}{features.Protocol, features.ModelFamily, features.FieldPaths, features.FieldProfiles, features.EventSequence, features.FinishReasons, features.HeaderPresence, features.HasSignatureID}
	encoded, _ := json.Marshal(signaturePayload)
	hash := sha256.Sum256(encoded)
	total := features.ProviderUsage.Total
	if total == 0 {
		total = features.UnifiedTokenCount
	}
	now := time.Now().Unix()
	for _, groupID := range groupIDs {
		sample := &model.ChannelPuritySample{
			GroupID: groupID, ChannelID: channelID, ActualModel: strings.TrimSpace(actualModel),
			RunKey: runKey, Protocol: features.Protocol, StructureSignature: hex.EncodeToString(hash[:]),
			StructureProfileJSON:  encodeFieldProfiles(features.FieldProfiles),
			StructureMetadataJSON: encodeStructureMetadata(features),
			PromptTokens:          features.ProviderUsage.Input, CompletionTokens: features.ProviderUsage.Output,
			TotalTokens: total, Valid: features.StatusCode >= 200 && features.StatusCode < 300 && !features.Truncated,
			ObservedAt: now,
		}
		if !sample.Valid {
			sample.ErrorClass = "invalid_anonymous_observation"
		}
		if err = model.CreatePuritySample(sample); err != nil {
			return err
		}
	}
	return nil
}
