package api

import (
	"github.com/felixbennettio/s-ui-next/service"
	"github.com/gin-gonic/gin"
)

func (a *ApiService) GetFilteredUsage(c *gin.Context) {
	result, err := a.StatsService.QueryUsage(service.UsageFilter{
		User: c.Query("user"), Search: c.Query("search"),
		Start: queryInt64(c, "start"), End: queryInt64(c, "end"),
		Offset: queryInt(c, "offset", 0), Limit: queryInt(c, "limit", 500),
	})
	jsonObj(c, result, err)
}

func (a *ApiService) GetFilteredStats(c *gin.Context) {
	result, err := a.StatsService.QueryStats(service.StatsFilter{
		Resource: c.Query("resource"), Tag: c.Query("tag"), Search: c.Query("search"),
		Start: queryInt64(c, "start"), End: queryInt64(c, "end"),
		Offset: queryInt(c, "offset", 0), Limit: queryInt(c, "limit", 2000),
	})
	jsonObj(c, result, err)
}

func (a *ApiService) GetConnectionAnalytics(c *gin.Context) {
	result, err := a.StatsService.QueryConnections(service.ConnectionFilter{
		Resource: c.Query("resource"), Tag: c.Query("tag"), User: c.Query("user"), Search: c.Query("search"),
		Start: queryInt64(c, "start"), End: queryInt64(c, "end"),
		Offset: queryInt(c, "offset", 0), Limit: queryInt(c, "limit", 500),
	})
	jsonObj(c, result, err)
}
