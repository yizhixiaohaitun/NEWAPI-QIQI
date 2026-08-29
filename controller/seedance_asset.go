package controller

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func RelaySeedanceAsset(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		respondSeedanceAssetError(c, service.TaskErrorWrapperLocal(err, "gen_relay_info_failed", http.StatusInternalServerError))
		return
	}

	var result any
	var taskErr *dto.TaskError
	retryParam := &service.RetryParam{
		Ctx:         c,
		TokenGroup:  relayInfo.TokenGroup,
		ModelName:   relayInfo.OriginModelName,
		RequestPath: c.Request.URL.Path,
		Retry:       common.GetPointer(0),
	}
	retryLimit := common.RetryTimes
	requestUpstreamCtx, cancelRequestUpstream := service.NewUpstreamRequestContext(c.Request.Context(), relayInfo.UpstreamTimeout)
	defer cancelRequestUpstream()

	for retryParam.GetRetry() <= retryLimit {
		if requestErr := c.Request.Context().Err(); requestErr != nil {
			taskErr = service.TaskErrorFromAPIError(service.NewClientClosedRequestError(requestErr))
			break
		}
		if errors.Is(requestUpstreamCtx.Err(), context.DeadlineExceeded) {
			taskErr = service.TaskErrorFromAPIError(service.NewUpstreamTimeoutError())
			break
		}
		channel, channelErr := getChannel(c, relayInfo, retryParam)
		if channelErr != nil {
			taskErr = service.TaskErrorWrapperLocal(channelErr.Err, "get_channel_failed", http.StatusServiceUnavailable)
			break
		}
		addUsedChannel(c, channel.Id)

		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			status := http.StatusBadRequest
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", status)
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		upstreamCtx, cancelUpstream := context.WithCancel(requestUpstreamCtx)
		relayInfo.UpstreamContext = upstreamCtx
		result, taskErr = relay.RelaySeedanceAsset(c, relayInfo)
		requestContextErr := c.Request.Context().Err()
		upstreamContextErr := upstreamCtx.Err()
		cancelUpstream()
		if requestContextErr != nil {
			taskErr = service.TaskErrorFromAPIError(service.NewClientClosedRequestError(requestContextErr))
		} else if errors.Is(upstreamContextErr, context.DeadlineExceeded) {
			taskErr = service.TaskErrorFromAPIError(service.NewUpstreamTimeoutError())
		}
		if taskErr == nil {
			break
		}

		if !taskErr.LocalError {
			processChannelError(c,
				*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey,
					common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()),
				types.NewOpenAIError(taskErr.Error, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode))
		}
		if !shouldRetryTaskRelay(c, channel.Id, taskErr, retryLimit-retryParam.GetRetry()) || !retryParam.IncreaseRetry() {
			break
		}
	}

	if taskErr != nil {
		respondSeedanceAssetError(c, taskErr)
		return
	}
	c.JSON(http.StatusOK, result)
}

func respondSeedanceAssetError(c *gin.Context, taskErr *dto.TaskError) {
	apiErr := types.NewOpenAIError(taskErr.Error, types.ErrorCode(taskErr.Code), taskErr.StatusCode)
	apiErr = service.SanitizeFinalRelayError(apiErr)
	c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.ToOpenAIError()})
}
