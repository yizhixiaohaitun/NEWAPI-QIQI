package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetModelProbe(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	results, err := model.ListModelProbeResults(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"results": results,
		},
	})
}
