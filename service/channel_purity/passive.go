package channel_purity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
)

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
		Events        []string        `json:"events"`
		FinishReasons []string        `json:"finish_reasons"`
		Headers       map[string]bool `json:"headers"`
		Signature     bool            `json:"signature"`
	}{features.Protocol, features.ModelFamily, features.FieldPaths, features.EventSequence, features.FinishReasons, features.HeaderPresence, features.HasSignatureID}
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
			PromptTokens: features.ProviderUsage.Input, CompletionTokens: features.ProviderUsage.Output,
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
