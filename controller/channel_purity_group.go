package controller

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func purityGroupFromRequest(r dto.ChannelPurityGroupRequest) *model.ChannelPurityGroup {
	g := &model.ChannelPurityGroup{Name: r.Name, Enabled: r.Enabled, IntervalMinutes: r.IntervalMinutes}
	g.Members = make([]model.ChannelPurityMember, len(r.Members))
	for i, m := range r.Members {
		g.Members[i] = model.ChannelPurityMember{ChannelID: m.ChannelID, IsBaseline: m.IsBaseline}
	}
	return g
}
func CreateChannelPurityGroup(c *gin.Context) {
	var r dto.ChannelPurityGroupRequest
	if c.ShouldBindJSON(&r) != nil {
		c.JSON(400, gin.H{"success": false, "message": "invalid group"})
		return
	}
	g := purityGroupFromRequest(r)
	now := time.Now().Unix()
	g.CreatedAt = now
	g.UpdatedAt = now
	if err := model.CreatePurityGroup(g); err != nil {
		c.JSON(400, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": g})
}
func ListChannelPurityGroups(c *gin.Context) {
	v, e := model.ListPurityGroups()
	if e != nil {
		common.ApiError(c, e)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": v})
}
func GetChannelPurityGroup(c *gin.Context) {
	id, e := strconv.ParseUint(c.Param("group_id"), 10, 64)
	if e != nil || id == 0 {
		c.JSON(400, gin.H{"success": false, "message": "invalid group id"})
		return
	}
	v, e := model.GetPurityGroup(uint(id))
	if e != nil {
		purityGroupLookup(c, e)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": v})
}
func UpdateChannelPurityGroup(c *gin.Context) {
	id, e := strconv.ParseUint(c.Param("group_id"), 10, 64)
	if e != nil || id == 0 {
		c.JSON(400, gin.H{"success": false, "message": "invalid group id"})
		return
	}
	var r dto.ChannelPurityGroupRequest
	if c.ShouldBindJSON(&r) != nil {
		c.JSON(400, gin.H{"success": false, "message": "invalid group"})
		return
	}
	g := purityGroupFromRequest(r)
	g.ID = uint(id)
	g.UpdatedAt = time.Now().Unix()
	if e = model.UpdatePurityGroup(g); e != nil {
		c.JSON(400, gin.H{"success": false, "message": e.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": g})
}
func DeleteChannelPurityGroup(c *gin.Context) {
	id, e := strconv.ParseUint(c.Param("group_id"), 10, 64)
	if e != nil || id == 0 {
		c.JSON(400, gin.H{"success": false, "message": "invalid group id"})
		return
	}
	if e = model.DeletePurityGroup(uint(id)); e != nil {
		common.ApiError(c, e)
		return
	}
	c.JSON(200, gin.H{"success": true})
}
func CreateChannelPuritySample(c *gin.Context) {
	var r dto.ChannelPuritySampleRequest
	if c.ShouldBindJSON(&r) != nil || r.GroupID == 0 || r.ChannelID <= 0 || strings.TrimSpace(r.ActualModel) == "" {
		c.JSON(400, gin.H{"success": false, "message": "group_id, channel_id and actual_model are required"})
		return
	}
	g, e := model.GetPurityGroup(r.GroupID)
	if e != nil {
		purityGroupLookup(c, e)
		return
	}
	member := false
	for _, m := range g.Members {
		if m.ChannelID == r.ChannelID {
			member = true
			break
		}
	}
	if !member {
		c.JSON(400, gin.H{"success": false, "message": "channel is not a member of group"})
		return
	}
	if r.ObservedAt == 0 {
		r.ObservedAt = time.Now().Unix()
	}
	s := &model.ChannelPuritySample{GroupID: r.GroupID, ChannelID: r.ChannelID, ActualModel: strings.TrimSpace(r.ActualModel), StructureSignature: r.StructureSignature, PromptTokens: r.PromptTokens, CompletionTokens: r.CompletionTokens, TotalTokens: r.TotalTokens, Valid: r.Valid, ErrorClass: r.ErrorClass, ObservedAt: r.ObservedAt}
	if e = model.CreatePuritySample(s); e != nil {
		common.ApiError(c, e)
		return
	}
	c.JSON(201, gin.H{"success": true, "data": s})
}
func GetLatestChannelPurityAssessment(c *gin.Context) {
	gid, e := strconv.ParseUint(c.Param("group_id"), 10, 64)
	tid, e2 := strconv.Atoi(c.Query("target_channel_id"))
	actual := strings.TrimSpace(c.Query("actual_model"))
	if e != nil || e2 != nil || gid == 0 || tid <= 0 || actual == "" {
		c.JSON(400, gin.H{"success": false, "message": "group_id, target_channel_id and actual_model are required"})
		return
	}
	v, e := model.GetLatestPurityAssessment(uint(gid), tid, actual)
	if e != nil {
		purityGroupLookup(c, e)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": v})
}
func ListChannelPurityHistory(c *gin.Context) {
	gid, e := strconv.ParseUint(c.Param("group_id"), 10, 64)
	tid, e2 := strconv.Atoi(c.Query("target_channel_id"))
	actual := strings.TrimSpace(c.Query("actual_model"))
	if e != nil || e2 != nil || gid == 0 || tid <= 0 || actual == "" {
		c.JSON(400, gin.H{"success": false, "message": "group_id, target_channel_id and actual_model are required"})
		return
	}
	p := common.GetPageQuery(c)
	v, total, e := model.ListPurityPairRuns(uint(gid), tid, actual, p.GetStartIdx(), p.GetPageSize())
	if e != nil {
		common.ApiError(c, e)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"items": v, "total": total, "page": p.GetPage(), "page_size": p.GetPageSize()}})
}
func purityGroupLookup(c *gin.Context, e error) {
	if e == gorm.ErrRecordNotFound {
		c.JSON(404, gin.H{"success": false, "message": "not found"})
	} else {
		common.ApiError(c, e)
	}
}
