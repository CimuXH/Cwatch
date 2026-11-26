package controllers

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"backend/models"
	"backend/services"
	"backend/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func StoreUpload(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("videoid"), 10, strconv.IntSize)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的视频 ID"})
		return
	}
	if c.Request.ContentLength > utils.MaxVideoSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "文件大小超过500MB"})
		return
	}
	body := http.MaxBytesReader(c.Writer, c.Request.Body, utils.MaxVideoSize)
	defer body.Close()
	if err := videoService.StoreUpload(c.Request.Context(), c.GetString("username"), uint(id), body); err != nil {
		var limitErr *http.MaxBytesError
		status := http.StatusBadRequest
		if errors.As(err, &limitErr) {
			status = http.StatusRequestEntityTooLarge
		}
		if errors.Is(err, utils.ErrMediaUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": utils.ErrMediaUnavailable.Error()})
			return
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func ServeMedia(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("videoid"), 10, strconv.IntSize)
	if err != nil || id == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	video, err := utils.GetVideoByID(c.Request.Context(), uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.Status(http.StatusNotFound)
		} else {
			c.Status(http.StatusServiceUnavailable)
		}
		return
	}
	name := strings.TrimPrefix(c.Param("file"), "/")
	url := utils.MediaURL(uint(id), name)
	if video.Status != models.VideoStatusUploaded ||
		(url != video.URL && url != video.CoverURL && url != video.URL720p && url != video.URL1080p) {
		c.Status(http.StatusNotFound)
		return
	}
	file, modified, err := utils.OpenVideoMedia(c.Request.Context(), video, name)
	if err != nil {
		if errors.Is(err, utils.ErrMediaUnavailable) {
			c.Status(http.StatusServiceUnavailable)
		} else {
			c.Status(http.StatusNotFound)
		}
		return
	}
	defer file.Close()
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "no-cache")
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4":
		c.Header("Content-Type", "video/mp4")
	case ".webm":
		c.Header("Content-Type", "video/webm")
	case ".mov":
		c.Header("Content-Type", "video/quicktime")
	case ".avi":
		c.Header("Content-Type", "video/x-msvideo")
	case ".mkv":
		c.Header("Content-Type", "video/x-matroska")
	case ".jpg":
		c.Header("Content-Type", "image/jpeg")
	default:
		c.Header("Content-Type", "application/octet-stream")
	}
	http.ServeContent(c.Writer, c.Request, filepath.Base(name), modified, file)
}

func RetryProcessing(c *gin.Context) {
	var req services.ConfirmUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的视频 ID"})
		return
	}
	if err := videoService.RetryProcessing(c.Request.Context(), c.GetString("username"), req.VideoID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusAccepted)
}
