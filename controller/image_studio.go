package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type imageStudioErrorProjectionWriter struct {
	gin.ResponseWriter
	errorStatus int
}

func (writer *imageStudioErrorProjectionWriter) WriteHeader(statusCode int) {
	if statusCode >= http.StatusBadRequest {
		writer.errorStatus = statusCode
		return
	}
	writer.ResponseWriter.WriteHeader(statusCode)
}

func (writer *imageStudioErrorProjectionWriter) Write(data []byte) (int, error) {
	if writer.errorStatus != 0 {
		return len(data), nil
	}
	return writer.ResponseWriter.Write(data)
}

func (writer *imageStudioErrorProjectionWriter) WriteString(value string) (int, error) {
	if writer.errorStatus != 0 {
		return len(value), nil
	}
	return writer.ResponseWriter.WriteString(value)
}

func (writer *imageStudioErrorProjectionWriter) WriteHeaderNow() {
	if writer.errorStatus == 0 {
		writer.ResponseWriter.WriteHeaderNow()
	}
}

func (writer *imageStudioErrorProjectionWriter) Status() int {
	if writer.errorStatus != 0 {
		return writer.errorStatus
	}
	return writer.ResponseWriter.Status()
}

func (writer *imageStudioErrorProjectionWriter) Written() bool {
	return writer.errorStatus != 0 || writer.ResponseWriter.Written()
}

func ImageStudioErrorProjection() gin.HandlerFunc {
	return func(c *gin.Context) {
		originalWriter := c.Writer
		originalHeaders := originalWriter.Header().Clone()
		projectedWriter := &imageStudioErrorProjectionWriter{ResponseWriter: originalWriter}
		c.Writer = projectedWriter
		defer func() { c.Writer = originalWriter }()
		c.Next()
		if projectedWriter.errorStatus == 0 {
			return
		}
		c.Writer = originalWriter
		statusCode := projectedWriter.errorStatus
		if statusCode < http.StatusBadRequest || statusCode > 599 {
			statusCode = http.StatusBadGateway
		}
		for header := range originalWriter.Header() {
			originalWriter.Header().Del(header)
		}
		for header, values := range originalHeaders {
			originalWriter.Header()[header] = values
		}
		originalWriter.Header().Set("Content-Type", gin.MIMEJSON)
		c.JSON(statusCode, gin.H{"error": gin.H{"code": "IMAGE_STUDIO_GENERATION_FAILED", "message": "Image generation is unavailable. Please try again later."}})
	}
}

func ImageStudio(c *gin.Context) {
	if c.GetBool("use_access_token") {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{"code": "IMAGE_STUDIO_SESSION_REQUIRED", "message": "Please sign in to generate images."}})
		return
	}
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAIImage, nil, nil)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "IMAGE_STUDIO_UNAVAILABLE", "message": "Image generation is unavailable. Please try again later."}})
		return
	}
	userCache, err := model.GetUserCache(c.GetInt("id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "IMAGE_STUDIO_UNAVAILABLE", "message": "Image generation is unavailable. Please try again later."}})
		return
	}
	userCache.WriteContext(c)
	temporaryToken := &model.Token{UserId: c.GetInt("id"), Name: "image-studio", Group: relayInfo.UsingGroup}
	_ = middleware.SetupContextForToken(c, temporaryToken)

	// Distribute selected the first channel from the server-owned request body.
	// The existing retry guard treats this marker as a one-send endpoint.
	c.Set("specific_channel_id", strconv.Itoa(common.GetContextKeyInt(c, constant.ContextKeyChannelId)))
	Relay(c, types.RelayFormatOpenAIImage)
}
