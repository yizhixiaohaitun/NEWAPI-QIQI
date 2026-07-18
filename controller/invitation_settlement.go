package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func RetryInvitationSettlement(c *gin.Context) {
	inviteeId, err := strconv.Atoi(c.Param("invitee_id"))
	if err != nil || inviteeId <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "valid invitee_id is required",
		})
		return
	}

	settledNow, err := model.RetryInvitationSettlement(inviteeId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"invitee_id":  inviteeId,
			"settled_now": settledNow,
		},
	})
}

func RetryPendingInvitationSettlements(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	result := model.RetryInvitationSettlements(limit)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}
