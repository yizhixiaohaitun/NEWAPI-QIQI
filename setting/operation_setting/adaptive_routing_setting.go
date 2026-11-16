package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

type AdaptiveRoutingSetting struct {
	Enabled bool `json:"enabled"`
}

var adaptiveRoutingSetting = AdaptiveRoutingSetting{}

func init() {
	config.GlobalConfig.Register("adaptive_routing_setting", &adaptiveRoutingSetting)
}

func GetAdaptiveRoutingSetting() *AdaptiveRoutingSetting {
	return &adaptiveRoutingSetting
}
