package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"web2api/internal/service"
)

type ImageHandler struct {
	chatService *service.ChatService
}

func NewImageHandler(chatService *service.ChatService) *ImageHandler {
	return &ImageHandler{chatService: chatService}
}

func (h *ImageHandler) CreateImage(c *gin.Context) {
	body := map[string]any{}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&body); err != nil {
		writeError(c, &service.StatusError{Code: 400, Message: err.Error()})
		return
	}

	prompt := service.AsString(body["prompt"])
	if prompt == "" {
		writeError(c, service.BadRequest("prompt is required"))
		return
	}
	model := service.FirstNonEmpty(service.AsString(body["model"]), "gpt-4o")
	n, err := service.ParseImageCount(body["n"], 1)
	if err != nil {
		writeError(c, err)
		return
	}
	result, err := h.chatService.CreateImageCompletion(c.Request.Context(), prompt, model, n)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *ImageHandler) EditImage(c *gin.Context) {
	prompt := c.PostForm("prompt")
	model := c.DefaultPostForm("model", "gpt-image-1")
	n, _ := strconv.Atoi(c.DefaultPostForm("n", "1"))
	if n < 1 || n > 4 {
		writeError(c, service.BadRequest("n must be between 1 and 4"))
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		writeError(c, &service.StatusError{Code: 400, Message: err.Error()})
		return
	}

	files := form.File["image"]
	images := make([]service.InputImage, 0, len(files))
	for index, file := range files {
		src, err := file.Open()
		if err != nil {
			writeError(c, err)
			return
		}
		data, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			writeError(c, err)
			return
		}
		if len(data) == 0 {
			writeError(c, service.BadRequest("image file is empty"))
			return
		}

		fileName := file.Filename
		if fileName == "" {
			fileName = "image.png"
		}
		mimeType := file.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = "image/png"
		}
		images = append(images, service.NewInputImage(data, fileName, mimeType, index))
	}

	result, err := h.chatService.EditImageCompletion(c.Request.Context(), prompt, model, n, images)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *ImageHandler) CreateChatCompletion(c *gin.Context) {
	body := map[string]any{}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, &service.StatusError{Code: 400, Message: err.Error()})
		return
	}

	result, err := h.chatService.CreateChatCompletion(c.Request.Context(), body)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *ImageHandler) CreateResponse(c *gin.Context) {
	body := map[string]any{}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, &service.StatusError{Code: 400, Message: err.Error()})
		return
	}

	result, err := h.chatService.CreateResponse(c.Request.Context(), body)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
