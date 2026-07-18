package channel_purity

import "github.com/QuantumNous/new-api/model"

const (
	AlertStatusOpen     = "OPEN"
	AlertStatusResolved = "RESOLVED"
)

func AlertTransition(previous, next string) (open, resolve bool) {
	open = previous != model.ChannelPurityStateAlert && next == model.ChannelPurityStateAlert
	resolve = previous == model.ChannelPurityStateAlert && next != model.ChannelPurityStateAlert
	return
}
