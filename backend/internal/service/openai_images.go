package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha3"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math/rand"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/imroc/req/v3"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAIImagesGenerationsEndpoint = "/v1/images/generations"
	openAIImagesEditsEndpoint       = "/v1/images/edits"

	openAIImagesGenerationsURL = "https://api.openai.com/v1/images/generations"
	openAIImagesEditsURL       = "https://api.openai.com/v1/images/edits"

	openAIChatGPTStartURL           = "https://chatgpt.com/"
	openAIChatGPTFilesURL           = "https://chatgpt.com/backend-api/files"
	openAIChatGPTPrepareURL         = "https://chatgpt.com/backend-api/f/conversation/prepare"
	openAIChatGPTConversationURL    = "https://chatgpt.com/backend-api/f/conversation"
	openAIChatGPTRequirementsURL    = "https://chatgpt.com/backend-api/sentinel/chat-requirements"
	openAIImageBackendUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	openAIImageMaxDownloadBytes     = 20 << 20 // 20MB per image download
	openAIImageMaxUploadPartSize    = 20 << 20 // 20MB per multipart upload part
	openAIImagesResponsesMainModel  = "gpt-5.4-mini"
	openAICodexImageGenerationModel = "codex-gpt-image-2"
	openAIDefaultClientVersion      = "prod-be885abbfcfe7b1f511e88b3003d9ee44757fbad"
	openAIDefaultClientBuildNumber  = "5955942"
)

const openAIFreeImageConversationModel = "gpt-5-3"
const openAIDefaultSentinelPowScript = "https://chatgpt.com/backend-api/sentinel/sdk.js"

type OpenAIImagesCapability string

const (
	OpenAIImagesCapabilityBasic  OpenAIImagesCapability = "images-basic"
	OpenAIImagesCapabilityNative OpenAIImagesCapability = "images-native"
)

type OpenAIImagesUpload struct {
	FieldName   string
	FileName    string
	ContentType string
	Data        []byte
	Width       int
	Height      int
}

type OpenAIImagesRequest struct {
	Endpoint           string
	ContentType        string
	Multipart          bool
	Model              string
	ExplicitModel      bool
	Prompt             string
	Stream             bool
	N                  int
	Size               string
	ExplicitSize       bool
	SizeTier           string
	ResponseFormat     string
	Quality            string
	Background         string
	OutputFormat       string
	Moderation         string
	InputFidelity      string
	Style              string
	OutputCompression  *int
	PartialImages      *int
	HasMask            bool
	HasNativeOptions   bool
	RequiredCapability OpenAIImagesCapability
	InputImageURLs     []string
	MaskImageURL       string
	Uploads            []OpenAIImagesUpload
	MaskUpload         *OpenAIImagesUpload
	Body               []byte
	bodyHash           string
}

func (r *OpenAIImagesRequest) ModerationBody() []byte {
	if r == nil {
		return nil
	}
	payload := map[string]any{}
	if prompt := strings.TrimSpace(r.Prompt); prompt != "" {
		payload["prompt"] = prompt
	}
	images := r.moderationImages()
	if len(images) > 0 {
		payload["images"] = images
	}
	if len(payload) == 0 {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return body
}

func (r *OpenAIImagesRequest) moderationImages() []map[string]string {
	if r == nil {
		return nil
	}
	images := make([]map[string]string, 0, len(r.InputImageURLs)+len(r.Uploads)+1)
	for _, imageURL := range r.InputImageURLs {
		imageURL = strings.TrimSpace(imageURL)
		if imageURL != "" {
			images = append(images, map[string]string{"image_url": imageURL})
		}
	}
	for _, upload := range r.Uploads {
		if dataURL := upload.ModerationDataURL(); dataURL != "" {
			images = append(images, map[string]string{"image_url": dataURL})
		}
	}
	if maskURL := strings.TrimSpace(r.MaskImageURL); maskURL != "" {
		images = append(images, map[string]string{"image_url": maskURL})
	}
	if r.MaskUpload != nil {
		if dataURL := r.MaskUpload.ModerationDataURL(); dataURL != "" {
			images = append(images, map[string]string{"image_url": dataURL})
		}
	}
	return images
}

func (u OpenAIImagesUpload) ModerationDataURL() string {
	if len(u.Data) == 0 {
		return ""
	}
	contentType := strings.TrimSpace(u.ContentType)
	if contentType == "" {
		contentType = http.DetectContentType(u.Data)
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return ""
	}
	return fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(u.Data))
}

type openAIChatRequirements struct {
	Token          string
	ProofToken     string
	TurnstileToken string
}

type openAIFreeImageConversationState struct {
	Text           string
	ConversationID string
	FileIDs        []string
	SedimentIDs    []string
	Blocked        bool
	ToolInvoked    *bool
	TurnUseCase    string
}

type openAIImageBridgeCallResult struct {
	Body                 []byte
	ImageCount           int
	ImageSize            string
	IgnoredBridgeParams  []string
	BridgeTarget         string
	BridgeDurationMillis int64
	Usage                OpenAIUsage
}

type openAIBridgeAssetRef struct {
	AssetID        string
	Filename       string
	OutputFormat   string
	RevisedPrompt  string
	ProtectedUntil string
}

type openAIPowScriptParser struct {
	scriptSources []string
	dataBuild     string
}

type openAITurnstileOrderedMap struct {
	keys   []string
	values map[string]any
}

func (r *OpenAIImagesRequest) IsEdits() bool {
	return r != nil && r.Endpoint == openAIImagesEditsEndpoint
}

func (r *OpenAIImagesRequest) StickySessionSeed() string {
	if r == nil {
		return ""
	}
	parts := []string{
		"openai-images",
		strings.TrimSpace(r.Endpoint),
		strings.TrimSpace(r.Model),
		strings.TrimSpace(r.Size),
		strings.TrimSpace(r.Prompt),
	}
	seed := strings.Join(parts, "|")
	if strings.TrimSpace(r.Prompt) == "" && r.bodyHash != "" {
		seed += "|body=" + r.bodyHash
	}
	return seed
}

func (s *OpenAIGatewayService) ParseOpenAIImagesRequest(c *gin.Context, body []byte) (*OpenAIImagesRequest, error) {
	if c == nil || c.Request == nil {
		return nil, fmt.Errorf("missing request context")
	}
	endpoint := normalizeOpenAIImagesEndpointPath(c.Request.URL.Path)
	if endpoint == "" {
		return nil, fmt.Errorf("unsupported images endpoint")
	}

	contentType := strings.TrimSpace(c.GetHeader("Content-Type"))
	req := &OpenAIImagesRequest{
		Endpoint:    endpoint,
		ContentType: contentType,
		N:           1,
		Body:        body,
	}
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		req.bodyHash = hex.EncodeToString(sum[:8])
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		req.Multipart = true
		if parseErr := parseOpenAIImagesMultipartRequest(body, contentType, req); parseErr != nil {
			return nil, parseErr
		}
	} else {
		if len(body) == 0 {
			return nil, fmt.Errorf("request body is empty")
		}
		if !gjson.ValidBytes(body) {
			return nil, fmt.Errorf("failed to parse request body")
		}
		if parseErr := parseOpenAIImagesJSONRequest(body, req); parseErr != nil {
			return nil, parseErr
		}
	}

	applyOpenAIImagesDefaults(req)
	if err := validateOpenAIImagesModel(req.Model); err != nil {
		return nil, err
	}
	req.SizeTier = normalizeOpenAIImageSizeTier(req.Size)
	req.RequiredCapability = classifyOpenAIImagesCapability(req)
	return req, nil
}

func parseOpenAIImagesJSONRequest(body []byte, req *OpenAIImagesRequest) error {
	if modelResult := gjson.GetBytes(body, "model"); modelResult.Exists() {
		req.Model = strings.TrimSpace(modelResult.String())
		req.ExplicitModel = req.Model != ""
	}
	req.Prompt = strings.TrimSpace(gjson.GetBytes(body, "prompt").String())

	if streamResult := gjson.GetBytes(body, "stream"); streamResult.Exists() {
		if streamResult.Type != gjson.True && streamResult.Type != gjson.False {
			return fmt.Errorf("invalid stream field type")
		}
		req.Stream = streamResult.Bool()
	}

	if nResult := gjson.GetBytes(body, "n"); nResult.Exists() {
		if nResult.Type != gjson.Number {
			return fmt.Errorf("invalid n field type")
		}
		req.N = int(nResult.Int())
		if req.N <= 0 {
			return fmt.Errorf("n must be greater than 0")
		}
	}

	if sizeResult := gjson.GetBytes(body, "size"); sizeResult.Exists() {
		req.Size = strings.TrimSpace(sizeResult.String())
		req.ExplicitSize = req.Size != ""
	}
	req.ResponseFormat = strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "response_format").String()))
	req.Quality = strings.TrimSpace(gjson.GetBytes(body, "quality").String())
	req.Background = strings.TrimSpace(gjson.GetBytes(body, "background").String())
	req.OutputFormat = strings.TrimSpace(gjson.GetBytes(body, "output_format").String())
	req.Moderation = strings.TrimSpace(gjson.GetBytes(body, "moderation").String())
	req.InputFidelity = strings.TrimSpace(gjson.GetBytes(body, "input_fidelity").String())
	req.Style = strings.TrimSpace(gjson.GetBytes(body, "style").String())
	req.HasMask = gjson.GetBytes(body, "mask").Exists()
	if outputCompression := gjson.GetBytes(body, "output_compression"); outputCompression.Exists() {
		if outputCompression.Type != gjson.Number {
			return fmt.Errorf("invalid output_compression field type")
		}
		v := int(outputCompression.Int())
		req.OutputCompression = &v
	}
	if partialImages := gjson.GetBytes(body, "partial_images"); partialImages.Exists() {
		if partialImages.Type != gjson.Number {
			return fmt.Errorf("invalid partial_images field type")
		}
		v := int(partialImages.Int())
		req.PartialImages = &v
	}
	if req.IsEdits() {
		images := gjson.GetBytes(body, "images")
		if images.Exists() {
			if !images.IsArray() {
				return fmt.Errorf("invalid images field type")
			}
			for _, item := range images.Array() {
				if imageURL := strings.TrimSpace(item.Get("image_url").String()); imageURL != "" {
					req.InputImageURLs = append(req.InputImageURLs, imageURL)
					continue
				}
				if item.Get("file_id").Exists() {
					return fmt.Errorf("images[].file_id is not supported (use images[].image_url instead)")
				}
			}
		}
		if maskImageURL := strings.TrimSpace(gjson.GetBytes(body, "mask.image_url").String()); maskImageURL != "" {
			req.MaskImageURL = maskImageURL
			req.HasMask = true
		}
		if gjson.GetBytes(body, "mask.file_id").Exists() {
			return fmt.Errorf("mask.file_id is not supported (use mask.image_url instead)")
		}
		if len(req.InputImageURLs) == 0 {
			return fmt.Errorf("images[].image_url is required")
		}
	}
	req.HasNativeOptions = hasOpenAINativeImageOptions(func(path string) bool {
		return gjson.GetBytes(body, path).Exists()
	})
	return nil
}

func parseOpenAIImagesMultipartRequest(body []byte, contentType string, req *OpenAIImagesRequest) error {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("invalid multipart content-type: %w", err)
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return fmt.Errorf("multipart boundary is required")
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read multipart body: %w", err)
		}
		name := strings.TrimSpace(part.FormName())
		if name == "" {
			_ = part.Close()
			continue
		}

		data, err := io.ReadAll(io.LimitReader(part, openAIImageMaxUploadPartSize))
		_ = part.Close()
		if err != nil {
			return fmt.Errorf("read multipart field %s: %w", name, err)
		}

		fileName := strings.TrimSpace(part.FileName())
		if fileName != "" {
			partContentType := strings.TrimSpace(part.Header.Get("Content-Type"))
			if name == "mask" && len(data) > 0 {
				req.HasMask = true
				width, height := parseOpenAIImageDimensions(part.Header)
				maskUpload := OpenAIImagesUpload{
					FieldName:   name,
					FileName:    fileName,
					ContentType: partContentType,
					Data:        data,
					Width:       width,
					Height:      height,
				}
				req.MaskUpload = &maskUpload
			}
			if name == "image" || strings.HasPrefix(name, "image[") {
				width, height := parseOpenAIImageDimensions(part.Header)
				req.Uploads = append(req.Uploads, OpenAIImagesUpload{
					FieldName:   name,
					FileName:    fileName,
					ContentType: partContentType,
					Data:        data,
					Width:       width,
					Height:      height,
				})
			}
			continue
		}

		value := strings.TrimSpace(string(data))
		switch name {
		case "model":
			req.Model = value
			req.ExplicitModel = value != ""
		case "prompt":
			req.Prompt = value
		case "size":
			req.Size = value
			req.ExplicitSize = value != ""
		case "response_format":
			req.ResponseFormat = strings.ToLower(value)
		case "stream":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid stream field value")
			}
			req.Stream = parsed
		case "n":
			n, err := strconv.Atoi(value)
			if err != nil || n <= 0 {
				return fmt.Errorf("n must be a positive integer")
			}
			req.N = n
		case "quality":
			req.Quality = value
			req.HasNativeOptions = true
		case "background":
			req.Background = value
			req.HasNativeOptions = true
		case "output_format":
			req.OutputFormat = value
			req.HasNativeOptions = true
		case "moderation":
			req.Moderation = value
			req.HasNativeOptions = true
		case "input_fidelity":
			req.InputFidelity = value
			req.HasNativeOptions = true
		case "style":
			req.Style = value
			req.HasNativeOptions = true
		case "output_compression":
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid output_compression field value")
			}
			req.OutputCompression = &n
			req.HasNativeOptions = true
		case "partial_images":
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid partial_images field value")
			}
			req.PartialImages = &n
			req.HasNativeOptions = true
		default:
			if isOpenAINativeImageOption(name) && value != "" {
				req.HasNativeOptions = true
			}
		}
	}

	if len(req.Uploads) == 0 && req.IsEdits() {
		return fmt.Errorf("image file is required")
	}
	return nil
}

func parseOpenAIImageDimensions(_ textproto.MIMEHeader) (int, int) {
	return 0, 0
}

func applyOpenAIImagesDefaults(req *OpenAIImagesRequest) {
	if req == nil {
		return
	}
	if req.N <= 0 {
		req.N = 1
	}
	if strings.TrimSpace(req.Model) != "" {
		req.Model = strings.TrimSpace(req.Model)
		return
	}
	req.Model = "gpt-image-2"
}

func isOpenAIImageGenerationModel(model string) bool {
	return IsGPTImageGenerationModel(model)
}

// IsGPTImageGenerationModel identifies the GPT native image-generation model family.
func IsGPTImageGenerationModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(model, "gpt-image-")
}

func validateOpenAIImagesModel(model string) error {
	model = strings.TrimSpace(model)
	if isOpenAIImageGenerationModel(model) {
		return nil
	}
	if model == "" {
		return fmt.Errorf("images endpoint requires an image model")
	}
	return fmt.Errorf("images endpoint requires an image model, got %q", model)
}

func normalizeOpenAIImagesEndpointPath(path string) string {
	trimmed := strings.TrimSpace(path)
	switch {
	case strings.Contains(trimmed, "/images/generations"):
		return openAIImagesGenerationsEndpoint
	case strings.Contains(trimmed, "/images/edits"):
		return openAIImagesEditsEndpoint
	default:
		return ""
	}
}

func classifyOpenAIImagesCapability(req *OpenAIImagesRequest) OpenAIImagesCapability {
	if req == nil {
		return OpenAIImagesCapabilityNative
	}
	if req.ExplicitModel || req.ExplicitSize {
		return OpenAIImagesCapabilityNative
	}
	model := strings.ToLower(strings.TrimSpace(req.Model))
	if !strings.HasPrefix(model, "gpt-image-") {
		return OpenAIImagesCapabilityNative
	}
	if req.Stream || req.N != 1 || req.HasMask || req.HasNativeOptions {
		return OpenAIImagesCapabilityNative
	}
	if req.IsEdits() && !req.Multipart {
		return OpenAIImagesCapabilityNative
	}
	if req.ResponseFormat != "" && req.ResponseFormat != "b64_json" {
		return OpenAIImagesCapabilityNative
	}
	return OpenAIImagesCapabilityBasic
}

func hasOpenAINativeImageOptions(exists func(path string) bool) bool {
	for _, path := range []string{
		"background",
		"quality",
		"style",
		"output_format",
		"output_compression",
		"moderation",
		"input_fidelity",
		"partial_images",
	} {
		if exists(path) {
			return true
		}
	}
	return false
}

func isOpenAINativeImageOption(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "background", "quality", "style", "output_format", "output_compression", "moderation", "input_fidelity", "partial_images":
		return true
	default:
		return false
	}
}

func normalizeOpenAIImageSizeTier(size string) string {
	trimmed := strings.TrimSpace(size)
	normalized := strings.ToLower(trimmed)
	switch normalized {
	case "", "auto":
		return "2K"
	case "1024x1024":
		return "1K"
	case "1536x1024", "1024x1536", "1792x1024", "1024x1792", "2048x2048", "2048x1152", "1152x2048":
		return "2K"
	case "3840x2160", "2160x3840":
		return "4K"
	}
	width, height, ok := parseOpenAIImageSizeDimensions(trimmed)
	if !ok {
		return "2K"
	}
	return classifyUnknownOpenAIImageSizeTier(width, height)
}

func resolveOpenAIImagesRequestModel(inboundModel string, channelMappedModel string) string {
	requestModel := strings.TrimSpace(inboundModel)
	mapped := strings.TrimSpace(channelMappedModel)
	if isOpenAIImageGenerationModel(mapped) {
		requestModel = mapped
	}
	if requestModel == "" {
		requestModel = "gpt-image-2"
	}
	return requestModel
}

func (s *OpenAIGatewayService) getOpenAIFreeImageBridgeURL(ctx context.Context) string {
	if s == nil || s.settingService == nil {
		return ""
	}
	settings, err := s.settingService.GetAllSettings(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(settings.OpenAIFreeImageBridgeURL), "/")
}

func (s *OpenAIGatewayService) getOpenAIFreeImageBridgeAuthKey(ctx context.Context) string {
	if s == nil || s.settingService == nil {
		return ""
	}
	settings, err := s.settingService.GetAllSettings(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(settings.OpenAIFreeImageBridgeAuthKey)
}

func buildOpenAIImageBridgePayload(parsed *OpenAIImagesRequest) (map[string]any, []string, error) {
	if parsed == nil {
		return nil, nil, fmt.Errorf("parsed images request is required")
	}
	payload := map[string]any{
		"prompt":          strings.TrimSpace(parsed.Prompt),
		"model":           strings.TrimSpace(parsed.Model),
		"n":               parsed.N,
		"size":            strings.TrimSpace(parsed.Size),
		"response_format": strings.TrimSpace(parsed.ResponseFormat),
		"stream":          parsed.Stream,
	}
	ignored := make([]string, 0, 8)
	if parsed.Quality != "" {
		payload["quality"] = strings.TrimSpace(parsed.Quality)
	}
	if parsed.Background != "" {
		payload["background"] = strings.TrimSpace(parsed.Background)
	}
	if parsed.Moderation != "" {
		ignored = append(ignored, "moderation")
	}
	if parsed.OutputFormat != "" {
		ignored = append(ignored, "output_format")
	}
	if parsed.OutputCompression != nil {
		ignored = append(ignored, "output_compression")
	}
	if parsed.PartialImages != nil {
		ignored = append(ignored, "partial_images")
	}
	if parsed.MaskUpload != nil || strings.TrimSpace(parsed.MaskImageURL) != "" || parsed.HasMask {
		ignored = append(ignored, "mask")
	}
	payload["bridge_mode"] = "sub2api"
	payload["bridge_transport"] = "asset_ref"
	return payload, ignored, nil
}

func postOpenAIImageBridgeGeneration(
	ctx context.Context,
	bridgeURL string,
	bridgeAuthKey string,
	parsed *OpenAIImagesRequest,
) (*openAIImageBridgeCallResult, error) {
	payload, ignored, err := buildOpenAIImageBridgePayload(parsed)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal image bridge payload: %w", err)
	}
	client := req.C().SetTimeout(5 * time.Minute)
	start := time.Now()
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", "Bearer "+bridgeAuthKey).
		SetBody(body).
		Post(bridgeURL + openAIImagesGenerationsEndpoint)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if !resp.IsSuccessState() {
		return nil, newOpenAIImageStatusError(resp, "image bridge generation failed")
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	return &openAIImageBridgeCallResult{
		Body:                 respBody,
		ImageCount:           len(gjson.GetBytes(respBody, "data").Array()),
		ImageSize:            parsed.SizeTier,
		IgnoredBridgeParams:  append(ignored, extractOpenAIBridgeIgnoredParams(respBody)...),
		BridgeTarget:         bridgeURL,
		BridgeDurationMillis: time.Since(start).Milliseconds(),
		Usage:                extractOpenAIBridgeUsage(respBody),
	}, nil
}

func postOpenAIImageBridgeEdit(
	ctx context.Context,
	bridgeURL string,
	bridgeAuthKey string,
	parsed *OpenAIImagesRequest,
) (*openAIImageBridgeCallResult, error) {
	payload, ignored, err := buildOpenAIImageBridgePayload(parsed)
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range payload {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				_ = writer.WriteField(key, typed)
			}
		case int:
			_ = writer.WriteField(key, strconv.Itoa(typed))
		case bool:
			_ = writer.WriteField(key, strconv.FormatBool(typed))
		}
	}
	for _, upload := range parsed.Uploads {
		part, err := writer.CreateFormFile("image[]", upload.FileName)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(upload.Data); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	client := req.C().SetTimeout(5 * time.Minute)
	start := time.Now()
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Content-Type", writer.FormDataContentType()).
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", "Bearer "+bridgeAuthKey).
		SetBody(body.Bytes()).
		Post(bridgeURL + openAIImagesEditsEndpoint)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if !resp.IsSuccessState() {
		return nil, newOpenAIImageStatusError(resp, "image bridge edit failed")
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	return &openAIImageBridgeCallResult{
		Body:                 respBody,
		ImageCount:           len(gjson.GetBytes(respBody, "data").Array()),
		ImageSize:            parsed.SizeTier,
		IgnoredBridgeParams:  append(ignored, extractOpenAIBridgeIgnoredParams(respBody)...),
		BridgeTarget:         bridgeURL,
		BridgeDurationMillis: time.Since(start).Milliseconds(),
		Usage:                extractOpenAIBridgeUsage(respBody),
	}, nil
}

func extractOpenAIBridgeUsage(body []byte) OpenAIUsage {
	usage := gjson.GetBytes(body, "usage")
	if !usage.Exists() {
		return OpenAIUsage{}
	}
	return OpenAIUsage{
		InputTokens:       int(usage.Get("input_tokens").Int()),
		OutputTokens:      int(usage.Get("output_tokens").Int()),
		ImageOutputTokens: int(usage.Get("output_tokens").Int()),
	}
}

func extractOpenAIBridgeIgnoredParams(body []byte) []string {
	meta := gjson.GetBytes(body, "meta.ignored_params")
	if !meta.Exists() || !meta.IsArray() {
		return nil
	}
	values := make([]string, 0, len(meta.Array()))
	for _, item := range meta.Array() {
		if trimmed := strings.TrimSpace(item.String()); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func extractOpenAIBridgeAssetRefs(body []byte) []openAIBridgeAssetRef {
	items := gjson.GetBytes(body, "data")
	if !items.Exists() || !items.IsArray() {
		return nil
	}
	out := make([]openAIBridgeAssetRef, 0, len(items.Array()))
	for _, item := range items.Array() {
		assetID := strings.TrimSpace(item.Get("asset_id").String())
		if assetID == "" {
			continue
		}
		out = append(out, openAIBridgeAssetRef{
			AssetID:        assetID,
			Filename:       strings.TrimSpace(item.Get("filename").String()),
			OutputFormat:   strings.TrimSpace(item.Get("output_format").String()),
			RevisedPrompt:  strings.TrimSpace(item.Get("revised_prompt").String()),
			ProtectedUntil: strings.TrimSpace(item.Get("protected_until").String()),
		})
	}
	return out
}

func fetchOpenAIBridgeAssetContent(
	ctx context.Context,
	bridgeURL string,
	bridgeAuthKey string,
	assetID string,
) ([]byte, string, error) {
	client := req.C().SetTimeout(5 * time.Minute)
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/octet-stream").
		SetHeader("Authorization", "Bearer "+bridgeAuthKey).
		DisableAutoReadResponse().
		Get(strings.TrimRight(bridgeURL, "/") + "/bridge/assets/" + assetID + "/content")
	if err != nil {
		return nil, "", err
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if !resp.IsSuccessState() {
		return nil, "", newOpenAIImageStatusError(resp, "fetch image bridge asset content failed")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, openAIImageMaxDownloadBytes))
	if err != nil {
		return nil, "", err
	}
	return body, strings.TrimSpace(resp.GetHeader("Content-Type")), nil
}

func deleteOpenAIBridgeAssetsAsync(
	bridgeURL string,
	bridgeAuthKey string,
	assetIDs []string,
) {
	ids := make([]string, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		if trimmed := strings.TrimSpace(assetID); trimmed != "" {
			ids = append(ids, trimmed)
		}
	}
	if len(ids) == 0 {
		return
	}
	go func() {
		time.Sleep(3 * time.Second)
		client := req.C().SetTimeout(30 * time.Second)
		payload := map[string]any{
			"asset_ids": ids,
			"reason":    "sub2api_response_completed",
		}
		resp, err := client.R().
			SetHeader("Content-Type", "application/json").
			SetHeader("Accept", "application/json").
			SetHeader("Authorization", "Bearer "+bridgeAuthKey).
			SetBodyJsonMarshal(payload).
			Post(strings.TrimRight(bridgeURL, "/") + "/bridge/assets/batch-delete")
		if err != nil {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Free image bridge async delete failed asset_ids=%s err=%v", strings.Join(ids, ","), err)
			return
		}
		if resp != nil && !resp.IsSuccessState() {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Free image bridge async delete non-success asset_ids=%s status=%d", strings.Join(ids, ","), resp.StatusCode)
		}
	}()
}

func buildOpenAIBridgeB64JSONResponse(
	created int64,
	usage OpenAIUsage,
	assets []openAIBridgeAssetRef,
	contents [][]byte,
) ([]byte, int, error) {
	data := make([]map[string]any, 0, len(contents))
	for idx, content := range contents {
		entry := map[string]any{
			"b64_json": base64.StdEncoding.EncodeToString(content),
		}
		if idx < len(assets) {
			if revisedPrompt := strings.TrimSpace(assets[idx].RevisedPrompt); revisedPrompt != "" {
				entry["revised_prompt"] = revisedPrompt
			}
		}
		data = append(data, entry)
	}
	payload := map[string]any{
		"created": created,
		"data":    data,
	}
	if usage != (OpenAIUsage{}) {
		payload["usage"] = map[string]any{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	return body, len(data), nil
}

const (
	openAIImage2KMaxPixels = 2560 * 1440
)

func parseOpenAIImageSizeDimensions(size string) (int, int, bool) {
	trimmed := strings.TrimSpace(size)
	parts := strings.Split(strings.ToLower(trimmed), "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, false
	}
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func classifyUnknownOpenAIImageSizeTier(width int, height int) string {
	if height > 0 && width > openAIImage2KMaxPixels/height {
		return "4K"
	}
	return "2K"
}

func (s *OpenAIGatewayService) ForwardImages(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed images request is required")
	}
	switch account.Type {
	case AccountTypeAPIKey:
		return s.forwardOpenAIImagesAPIKey(ctx, c, account, body, parsed, channelMappedModel)
	case AccountTypeOAuth:
		if account.IsOpenAIOAuthFreePlan() {
			return s.forwardOpenAIImagesOAuthFree(ctx, c, account, parsed, channelMappedModel)
		}
		return s.forwardOpenAIImagesOAuth(ctx, c, account, parsed, channelMappedModel)
	default:
		return nil, fmt.Errorf("unsupported account type: %s", account.Type)
	}
}

func (s *OpenAIGatewayService) forwardOpenAIImagesAPIKey(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	requestModel := resolveOpenAIImagesRequestModel(parsed.Model, channelMappedModel)
	if err := validateOpenAIImagesModel(requestModel); err != nil {
		return nil, err
	}
	upstreamModel := account.GetMappedModel(requestModel)
	if err := validateOpenAIImagesModel(upstreamModel); err != nil {
		return nil, err
	}
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI] Images request routing request_model=%s upstream_model=%s endpoint=%s account_type=%s",
		strings.TrimSpace(parsed.Model),
		upstreamModel,
		parsed.Endpoint,
		account.Type,
	)
	forwardBody, forwardContentType, err := rewriteOpenAIImagesModel(body, parsed.ContentType, upstreamModel)
	if err != nil {
		return nil, err
	}
	if !parsed.Multipart {
		setOpsUpstreamRequestBody(c, forwardBody)
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()

	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := s.buildOpenAIImagesRequest(upstreamCtx, c, account, forwardBody, forwardContentType, token, parsed.Endpoint)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
			Kind:               "request_error",
			Message:            safeErr,
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		respBody = s.redactAgentIdentitySensitiveBody(upstreamCtx, account, respBody)
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			shouldDisable := s.handleFailoverSideEffects(upstreamCtx, resp, account, respBody, upstreamModel)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: !shouldDisable && account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleErrorResponse(upstreamCtx, resp, c, account, forwardBody)
	}
	defer func() { _ = resp.Body.Close() }()

	var usage OpenAIUsage
	imageCount := parsed.N
	var firstTokenMs *int
	if parsed.Stream && isEventStreamResponse(resp.Header) {
		streamUsage, streamCount, streamSizes, ttft, err := s.handleOpenAIImagesStreamingResponse(resp, c, startTime)
		if err != nil {
			if streamCount > 0 {
				return &OpenAIForwardResult{
					RequestID:        resp.Header.Get("x-request-id"),
					Usage:            streamUsage,
					Model:            requestModel,
					UpstreamModel:    upstreamModel,
					Stream:           parsed.Stream,
					ResponseHeaders:  resp.Header.Clone(),
					Duration:         time.Since(startTime),
					FirstTokenMs:     ttft,
					ImageCount:       streamCount,
					ImageSize:        parsed.SizeTier,
					ImageInputSize:   parsed.Size,
					ImageOutputSizes: streamSizes,
				}, err
			}
			return nil, err
		}
		usage = streamUsage
		imageCount = streamCount
		imageOutputSizes := streamSizes
		firstTokenMs = ttft
		return &OpenAIForwardResult{
			RequestID:        resp.Header.Get("x-request-id"),
			Usage:            usage,
			Model:            requestModel,
			UpstreamModel:    upstreamModel,
			Stream:           parsed.Stream,
			ResponseHeaders:  resp.Header.Clone(),
			Duration:         time.Since(startTime),
			FirstTokenMs:     firstTokenMs,
			ImageCount:       imageCount,
			ImageSize:        parsed.SizeTier,
			ImageInputSize:   parsed.Size,
			ImageOutputSizes: imageOutputSizes,
		}, nil
	} else {
		nonStreamUsage, nonStreamCount, nonStreamSizes, err := s.handleOpenAIImagesNonStreamingResponse(resp, c)
		if err != nil {
			return nil, err
		}
		usage = nonStreamUsage
		if nonStreamCount > 0 {
			imageCount = nonStreamCount
		}
		return &OpenAIForwardResult{
			RequestID:        resp.Header.Get("x-request-id"),
			Usage:            usage,
			Model:            requestModel,
			UpstreamModel:    upstreamModel,
			Stream:           parsed.Stream,
			ResponseHeaders:  resp.Header.Clone(),
			Duration:         time.Since(startTime),
			FirstTokenMs:     firstTokenMs,
			ImageCount:       imageCount,
			ImageSize:        parsed.SizeTier,
			ImageInputSize:   parsed.Size,
			ImageOutputSizes: nonStreamSizes,
		}, nil
	}
}

func (s *OpenAIGatewayService) forwardOpenAIImagesOAuthFree(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	requestModel := resolveOpenAIImagesRequestModel(parsed.Model, channelMappedModel)
	if err := validateOpenAIImagesModel(requestModel); err != nil {
		return nil, err
	}

	if bridgeURL := s.getOpenAIFreeImageBridgeURL(ctx); bridgeURL != "" {
		bridgeAuthKey := s.getOpenAIFreeImageBridgeAuthKey(ctx)
		if bridgeAuthKey == "" {
			return nil, fmt.Errorf("openai free image bridge auth key is not configured")
		}
		var (
			bridgeResult *openAIImageBridgeCallResult
			err          error
		)
		if parsed.IsEdits() {
			bridgeResult, err = postOpenAIImageBridgeEdit(ctx, bridgeURL, bridgeAuthKey, parsed)
		} else {
			bridgeResult, err = postOpenAIImageBridgeGeneration(ctx, bridgeURL, bridgeAuthKey, parsed)
		}
		if err != nil {
			return nil, err
		}
		assetRefs := extractOpenAIBridgeAssetRefs(bridgeResult.Body)
		if len(assetRefs) > 0 {
			assetIDs := make([]string, 0, len(assetRefs))
			for _, asset := range assetRefs {
				assetIDs = append(assetIDs, asset.AssetID)
			}
			logger.LegacyPrintf(
				"service.openai_gateway",
				"[OpenAI] Free image bridge asset refs received count=%d asset_ids=%s",
				len(assetRefs),
				strings.Join(assetIDs, ","),
			)
			contents := make([][]byte, 0, len(assetRefs))
			contentFetchStartedAt := time.Now()
			for _, asset := range assetRefs {
				content, _, fetchErr := fetchOpenAIBridgeAssetContent(ctx, bridgeURL, bridgeAuthKey, asset.AssetID)
				if fetchErr != nil {
					return nil, fetchErr
				}
				contents = append(contents, content)
				logger.LegacyPrintf(
					"service.openai_gateway",
					"[OpenAI] Free image bridge asset content fetched asset_id=%s bytes=%d",
					asset.AssetID,
					len(content),
				)
			}
			logger.LegacyPrintf(
				"service.openai_gateway",
				"[OpenAI] Free image bridge content fetch total_ms=%d asset_count=%d",
				time.Since(contentFetchStartedAt).Milliseconds(),
				len(contents),
			)

			if parsed.Stream {
				c.Status(http.StatusOK)
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				flusher, ok := c.Writer.(http.Flusher)
				if !ok {
					return nil, fmt.Errorf("streaming is not supported by response writer")
				}
				for idx, content := range contents {
					entry := map[string]any{
						"type":          "completed",
						"index":         idx,
						"b64_json":      base64.StdEncoding.EncodeToString(content),
						"output_format": firstNonEmptyString(assetRefs[idx].OutputFormat, "png"),
						"filename":      firstNonEmptyString(assetRefs[idx].Filename, fmt.Sprintf("image-%d.png", idx)),
					}
					if revisedPrompt := strings.TrimSpace(assetRefs[idx].RevisedPrompt); revisedPrompt != "" {
						entry["revised_prompt"] = revisedPrompt
					}
					if _, writeErr := io.WriteString(c.Writer, "data: "); writeErr != nil {
						return nil, writeErr
					}
					line, marshalErr := json.Marshal(entry)
					if marshalErr != nil {
						return nil, marshalErr
					}
					if _, writeErr := c.Writer.Write(line); writeErr != nil {
						return nil, writeErr
					}
					if _, writeErr := io.WriteString(c.Writer, "\n\n"); writeErr != nil {
						return nil, writeErr
					}
					flusher.Flush()
				}
				donePayload := map[string]any{
					"type": "done",
					"images": func() []map[string]any {
						out := make([]map[string]any, 0, len(contents))
						for idx, content := range contents {
							entry := map[string]any{
								"filename":      firstNonEmptyString(assetRefs[idx].Filename, fmt.Sprintf("image-%d.png", idx)),
								"output_format": firstNonEmptyString(assetRefs[idx].OutputFormat, "png"),
								"b64_json":      base64.StdEncoding.EncodeToString(content),
							}
							if revisedPrompt := strings.TrimSpace(assetRefs[idx].RevisedPrompt); revisedPrompt != "" {
								entry["revised_prompt"] = revisedPrompt
							}
							out = append(out, entry)
						}
						return out
					}(),
				}
				if _, writeErr := io.WriteString(c.Writer, "data: "); writeErr != nil {
					return nil, writeErr
				}
				line, marshalErr := json.Marshal(donePayload)
				if marshalErr != nil {
					return nil, marshalErr
				}
				if _, writeErr := c.Writer.Write(line); writeErr != nil {
					return nil, writeErr
				}
				if _, writeErr := io.WriteString(c.Writer, "\n\n"); writeErr != nil {
					return nil, writeErr
				}
				flusher.Flush()
				logger.LegacyPrintf(
					"service.openai_gateway",
					"[OpenAI] Free image bridge pseudo-stream completed scheduling delete asset_ids=%s",
					strings.Join(assetIDs, ","),
				)
				deleteOpenAIBridgeAssetsAsync(bridgeURL, bridgeAuthKey, assetIDs)
			} else {
				b64BuildStartedAt := time.Now()
				respBody, imageCount, buildErr := buildOpenAIBridgeB64JSONResponse(time.Now().Unix(), bridgeResult.Usage, assetRefs, contents)
				if buildErr != nil {
					return nil, buildErr
				}
				c.Header("Content-Type", "application/json")
				c.Status(http.StatusOK)
				c.Data(http.StatusOK, "application/json", respBody)
				bridgeResult.ImageCount = imageCount
				logger.LegacyPrintf(
					"service.openai_gateway",
					"[OpenAI] Free image bridge b64 response built image_count=%d build_ms=%d scheduling delete asset_ids=%s",
					imageCount,
					time.Since(b64BuildStartedAt).Milliseconds(),
					strings.Join(assetIDs, ","),
				)
				deleteOpenAIBridgeAssetsAsync(bridgeURL, bridgeAuthKey, assetIDs)
			}
		} else {
			c.Header("Content-Type", "application/json")
			c.Status(http.StatusOK)
			c.Data(http.StatusOK, "application/json", bridgeResult.Body)
		}
		logger.LegacyPrintf(
			"service.openai_gateway",
			"[OpenAI] Free image bridge used target=%s endpoint=%s duration_ms=%d ignored_params=%s",
			bridgeResult.BridgeTarget,
			parsed.Endpoint,
			bridgeResult.BridgeDurationMillis,
			strings.Join(bridgeResult.IgnoredBridgeParams, ","),
		)
		c.Set("openai_free_image_bridge_target", bridgeResult.BridgeTarget)
		c.Set("openai_free_image_bridge_duration_ms", bridgeResult.BridgeDurationMillis)
		c.Set("openai_free_image_bridge_ignored_params", bridgeResult.IgnoredBridgeParams)
		return &OpenAIForwardResult{
			RequestID:       "",
			Usage:           bridgeResult.Usage,
			Model:           requestModel,
			UpstreamModel:   requestModel,
			Stream:          parsed.Stream,
			ResponseHeaders: http.Header{},
			Duration:        time.Since(startTime),
			ImageCount:      bridgeResult.ImageCount,
			ImageSize:       bridgeResult.ImageSize,
			BillingTier: func() *string {
				tier := "free_image_bridge"
				return &tier
			}(),
		}, nil
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	client, err := createOpenAIFreeImageClient(proxyURL)
	if err != nil {
		return nil, err
	}

	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI] Images request routing request_model=%s endpoint=%s account_type=%s plan_type=%s route=free_conversation",
		requestModel,
		parsed.Endpoint,
		account.Type,
		account.OpenAIPlanType(),
	)

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()

	requirements, err := fetchOpenAIFreeImageRequirements(upstreamCtx, client, account)
	if err != nil {
		return nil, classifyOpenAIFreeImageSetupError(err, account)
	}
	conduitToken, err := prepareOpenAIFreeImageConversation(upstreamCtx, client, account, requirements, strings.TrimSpace(parsed.Prompt))
	if err != nil {
		return nil, classifyOpenAIFreeImageSetupError(err, account)
	}

	inputImages := make([]string, 0, len(parsed.InputImageURLs)+len(parsed.Uploads))
	for _, imageURL := range parsed.InputImageURLs {
		if trimmed := strings.TrimSpace(imageURL); trimmed != "" {
			inputImages = append(inputImages, trimmed)
		}
	}
	for _, upload := range parsed.Uploads {
		dataURL, dataErr := openAIImageUploadToDataURL(upload)
		if dataErr != nil {
			return nil, dataErr
		}
		inputImages = append(inputImages, dataURL)
	}
	uploadedRefs := make([]map[string]any, 0, len(inputImages))
	for idx, imageURL := range inputImages {
		fileName := fmt.Sprintf("image_%d.png", idx+1)
		ref, uploadErr := uploadOpenAIFreeImage(upstreamCtx, client, account, imageURL, fileName)
		if uploadErr != nil {
			return nil, classifyOpenAIFreeImageSetupError(uploadErr, account)
		}
		uploadedRefs = append(uploadedRefs, ref)
	}

	conversationBody, err := buildOpenAIImagesFreeConversationRequest(parsed, uploadedRefs)
	if err != nil {
		return nil, err
	}
	setOpsUpstreamRequestBody(c, conversationBody)
	upstreamStart := time.Now()
	rawBody, state, err := startOpenAIFreeImageConversation(upstreamCtx, client, account, requirements, conduitToken, conversationBody)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			UpstreamURL:        safeUpstreamURL(openAIChatGPTConversationURL),
			Kind:               "request_error",
			Message:            safeErr,
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	urls, err := s.pollOpenAIFreeImageResultURLs(upstreamCtx, account, state)
	if err != nil {
		return nil, classifyOpenAIFreeImageSetupError(err, account)
	}
	results, usage, err := s.handleOpenAIImagesOAuthFreeConversationBody(upstreamCtx, c, account, rawBody, state, urls, parsed)
	if err != nil {
		return nil, err
	}
	c.Status(http.StatusOK)
	c.Header("Content-Type", "application/json")
	c.Data(http.StatusOK, "application/json", results)

	return &OpenAIForwardResult{
		RequestID:       state.ConversationID,
		Usage:           usage,
		Model:           requestModel,
		UpstreamModel:   openAIFreeImageConversationModel,
		Stream:          parsed.Stream,
		ResponseHeaders: http.Header{},
		Duration:        time.Since(startTime),
		ImageCount:      extractOpenAIImagesBillableCountFromJSONBytes(results),
		ImageSize:       parsed.SizeTier,
	}, nil
}

func (s *OpenAIGatewayService) buildOpenAIImagesRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	contentType string,
	token string,
	endpoint string,
) (*http.Request, error) {
	targetURL := openAIImagesGenerationsURL
	if endpoint == openAIImagesEditsEndpoint {
		targetURL = openAIImagesEditsURL
	}
	baseURL := account.GetOpenAIBaseURL()
	if baseURL != "" {
		validatedURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, err
		}
		targetURL = buildOpenAIImagesURL(validatedURL, endpoint)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build openai authentication headers: %w", err)
	}
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	for key, values := range c.Request.Header {
		if !openaiPassthroughAllowedHeaders[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("User-Agent", customUA)
	}
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}
	// 账号级请求头覆写（仅 openai api_key 账号启用时生效；OAuth 路径 no-op）
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

func buildOpenAIImagesURL(base string, endpoint string) string {
	return buildOpenAIEndpointURL(base, endpoint)

}

func parseOpenAIPowResources(htmlContent string) ([]string, string) {
	parser := &openAIPowScriptParser{}
	scriptTagRe := regexp.MustCompile(`(?is)<script[^>]+src="([^"]+)"[^>]*>`)
	buildRe := regexp.MustCompile(`c/[^/]*/_`)
	for _, match := range scriptTagRe.FindAllStringSubmatch(htmlContent, -1) {
		if len(match) < 2 {
			continue
		}
		src := strings.TrimSpace(match[1])
		if src == "" {
			continue
		}
		parser.scriptSources = append(parser.scriptSources, src)
		if parser.dataBuild == "" {
			if hit := buildRe.FindString(src); hit != "" {
				parser.dataBuild = hit
			}
		}
	}
	if parser.dataBuild == "" {
		htmlBuildRe := regexp.MustCompile(`data-build="([^"]*)"`)
		if match := htmlBuildRe.FindStringSubmatch(htmlContent); len(match) > 1 {
			parser.dataBuild = strings.TrimSpace(match[1])
		}
	}
	if len(parser.scriptSources) == 0 {
		parser.scriptSources = []string{openAIDefaultSentinelPowScript}
	}
	return parser.scriptSources, parser.dataBuild
}

func buildOpenAILegacyRequirementsToken(userAgent string, scriptSources []string, dataBuild string) string {
	seed := strconv.FormatFloat(rand.Float64(), 'f', -1, 64)
	config := buildOpenAIPowConfig(userAgent, scriptSources, dataBuild)
	answer, _ := solveOpenAIPow(seed, "0fffff", config, 500000)
	return "gAAAAAC" + answer
}

func buildOpenAIProofToken(seed string, difficulty string, userAgent string, scriptSources []string, dataBuild string) (string, error) {
	config := buildOpenAIPowConfig(userAgent, scriptSources, dataBuild)
	answer, solved := solveOpenAIPow(seed, difficulty, config, 500000)
	if !solved {
		return "", fmt.Errorf("failed to solve proof token")
	}
	return "gAAAAAB" + answer, nil
}

func buildOpenAIPowConfig(userAgent string, scriptSources []string, dataBuild string) []any {
	now := time.Now().Add(-5 * time.Hour)
	scriptSource := openAIDefaultSentinelPowScript
	if len(scriptSources) > 0 {
		scriptSource = scriptSources[rand.Intn(len(scriptSources))]
	}
	return []any{
		3000 + rand.Intn(3)*1000,
		now.Format("Mon Jan 02 2006 15:04:05") + " GMT-0500 (Eastern Standard Time)",
		4294705152,
		0,
		userAgent,
		scriptSource,
		dataBuild,
		"en-US",
		"en-US,es-US,en,es",
		0,
		"webdriver−false",
		"location",
		"window",
		float64(time.Now().UnixNano()) / 1e6,
		uuid.NewString(),
		"",
		8,
		float64(time.Now().UnixNano()) / 1e6,
	}
}

func solveOpenAIPow(seed string, difficulty string, config []any, limit int) (string, bool) {
	target, err := hex.DecodeString(strings.TrimSpace(difficulty))
	if err != nil || len(target) == 0 {
		return "", false
	}
	if len(config) >= 18 {
		static1, err1 := json.Marshal(config[:3])
		static2, err2 := json.Marshal(config[4:9])
		static3, err3 := json.Marshal(config[10:])
		if err1 == nil && err2 == nil && err3 == nil && len(static1) > 0 && len(static2) > 0 && len(static3) > 0 {
			prefix := append([]byte{}, static1[:len(static1)-1]...)
			prefix = append(prefix, ',')
			mid := append([]byte{','}, static2[1:len(static2)-1]...)
			mid = append(mid, ',')
			suffix := append([]byte{','}, static3[1:]...)
			targetLen := len(strings.TrimSpace(difficulty)) / 2
			seedBytes := []byte(seed)
			for i := 0; i < limit; i++ {
				payload := make([]byte, 0, len(prefix)+len(mid)+len(suffix)+32)
				payload = append(payload, prefix...)
				payload = append(payload, strconv.Itoa(i)...)
				payload = append(payload, mid...)
				payload = append(payload, strconv.Itoa(i>>1)...)
				payload = append(payload, suffix...)
				encoded := base64.StdEncoding.EncodeToString(payload)
				sum := sha3.Sum512(append(seedBytes, []byte(encoded)...))
				if bytes.Compare(sum[:targetLen], target) <= 0 {
					return encoded, true
				}
			}
			fallback := "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
			return fallback, false
		}
	}
	configBytes, err := json.Marshal(config)
	if err != nil {
		return "", false
	}
	seedBytes := []byte(seed)
	for i := 0; i < limit; i++ {
		payload := append([]byte{}, configBytes...)
		payload = append(payload, []byte(strconv.Itoa(i))...)
		encoded := base64.StdEncoding.EncodeToString(payload)
		sum := sha3.Sum512(append(seedBytes, []byte(encoded)...))
		if bytes.Compare(sum[:len(target)], target) <= 0 {
			return encoded, true
		}
	}
	fallback := "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
	return fallback, false
}

func (m *openAITurnstileOrderedMap) add(key string, value any) {
	if m == nil {
		return
	}
	if m.values == nil {
		m.values = make(map[string]any)
	}
	if _, exists := m.values[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

func turnstileValueToString(value any) string {
	switch v := value.(type) {
	case nil:
		return "undefined"
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		switch v {
		case "window.Math":
			return "[object Math]"
		case "window.Reflect":
			return "[object Reflect]"
		case "window.performance":
			return "[object Performance]"
		case "window.localStorage":
			return "[object Storage]"
		case "window.Object":
			return "function Object() { [native code] }"
		case "window.Reflect.set":
			return "function set() { [native code] }"
		case "window.performance.now":
			return "function () { [native code] }"
		case "window.Object.create":
			return "function create() { [native code] }"
		case "window.Object.keys":
			return "function keys() { [native code] }"
		case "window.Math.random":
			return "function random() { [native code] }"
		default:
			return v
		}
	case []string:
		return strings.Join(v, ",")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, turnstileValueToString(item))
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprint(v)
	}
}

func xorOpenAITurnstileString(text string, key string) string {
	if key == "" {
		return text
	}
	keyRunes := []rune(key)
	out := make([]rune, 0, len(text))
	for i, ch := range []rune(text) {
		out = append(out, ch^keyRunes[i%len(keyRunes)])
	}
	return string(out)
}

func solveOpenAITurnstileToken(dx string, p string) string {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(dx))
	if err != nil {
		return ""
	}
	decoded := xorOpenAITurnstileString(string(raw), p)
	var tokenList []any
	if err := json.Unmarshal([]byte(decoded), &tokenList); err != nil {
		return ""
	}

	processMap := map[float64]any{
		10: "window",
		16: p,
	}
	startTime := time.Now()
	result := ""

	func1 := func(e float64, t float64) {
		processMap[e] = xorOpenAITurnstileString(turnstileValueToString(processMap[e]), turnstileValueToString(processMap[t]))
	}
	func2 := func(e float64, t any) {
		processMap[e] = t
	}
	func3 := func(e string) { result = base64.StdEncoding.EncodeToString([]byte(e)) }
	func5 := func(e float64, t float64) {
		current := processMap[e]
		incoming := processMap[t]
		switch cur := current.(type) {
		case []any:
			processMap[e] = append(cur, incoming)
		case []string:
			processMap[e] = append(cur, turnstileValueToString(incoming))
		default:
			if _, ok := current.(string); ok {
				processMap[e] = turnstileValueToString(current) + turnstileValueToString(incoming)
				return
			}
			if _, ok := current.(float64); ok {
				processMap[e] = turnstileValueToString(current) + turnstileValueToString(incoming)
				return
			}
			if _, ok := incoming.(string); ok {
				processMap[e] = turnstileValueToString(current) + turnstileValueToString(incoming)
				return
			}
			if _, ok := incoming.(float64); ok {
				processMap[e] = turnstileValueToString(current) + turnstileValueToString(incoming)
				return
			}
			processMap[e] = "NaN"
		}
	}
	func6 := func(e float64, t float64, n float64) {
		tv := turnstileValueToString(processMap[t])
		nv := turnstileValueToString(processMap[n])
		value := tv + "." + nv
		if value == "window.document.location" {
			processMap[e] = "https://chatgpt.com/"
			return
		}
		processMap[e] = value
	}
	func7 := func(e float64, args ...float64) {
		target := processMap[e]
		values := make([]any, 0, len(args))
		for _, arg := range args {
			values = append(values, processMap[arg])
		}
		if targetStr, ok := target.(string); ok && targetStr == "window.Reflect.set" && len(values) >= 3 {
			obj, _ := values[0].(*openAITurnstileOrderedMap)
			if obj != nil {
				obj.add(turnstileValueToString(values[1]), values[2])
			}
			return
		}
		if callable, ok := target.(func(...any)); ok {
			callable(values...)
		}
	}
	func8 := func(e float64, t float64) { processMap[e] = processMap[t] }
	func14 := func(e float64, t float64) {
		var parsed any
		if err := json.Unmarshal([]byte(turnstileValueToString(processMap[t])), &parsed); err == nil {
			processMap[e] = parsed
		}
	}
	func15 := func(e float64, t float64) {
		if b, err := json.Marshal(processMap[t]); err == nil {
			processMap[e] = string(b)
		}
	}
	func17 := func(e float64, t float64, args ...float64) {
		callArgs := make([]any, 0, len(args))
		for _, arg := range args {
			callArgs = append(callArgs, processMap[arg])
		}
		target := turnstileValueToString(processMap[t])
		switch target {
		case "window.performance.now":
			elapsed := float64(time.Since(startTime).Nanoseconds()) / 1e6
			processMap[e] = elapsed + rand.Float64()
		case "window.Object.create":
			processMap[e] = &openAITurnstileOrderedMap{values: make(map[string]any)}
		case "window.Object.keys":
			processMap[e] = []string{
				"STATSIG_LOCAL_STORAGE_INTERNAL_STORE_V4",
				"STATSIG_LOCAL_STORAGE_STABLE_ID",
				"client-correlated-secret",
				"oai/apps/capExpiresAt",
				"oai-did",
				"STATSIG_LOCAL_STORAGE_LOGGING_REQUEST",
				"UiState.isNavigationCollapsed.1",
			}
		case "window.Math.random":
			processMap[e] = rand.Float64()
		default:
			if callable, ok := processMap[t].(func(...any) any); ok {
				processMap[e] = callable(callArgs...)
			}
		}
	}
	func18 := func(e float64) {
		decodedValue, err := base64.StdEncoding.DecodeString(turnstileValueToString(processMap[e]))
		if err == nil {
			processMap[e] = string(decodedValue)
		}
	}
	func19 := func(e float64) {
		processMap[e] = base64.StdEncoding.EncodeToString([]byte(turnstileValueToString(processMap[e])))
	}
	func20 := func(e float64, t float64, n float64, args ...float64) {
		if turnstileValueToString(processMap[e]) == turnstileValueToString(processMap[t]) {
			if callable, ok := processMap[n].(func(...any)); ok {
				callArgs := make([]any, 0, len(args))
				for _, arg := range args {
					callArgs = append(callArgs, processMap[arg])
				}
				callable(callArgs...)
			}
		}
	}
	func21 := func(...any) {}
	func23 := func(e float64, t float64, args ...float64) {
		if processMap[e] != nil {
			if callable, ok := processMap[t].(func(...any)); ok {
				callArgs := make([]any, 0, len(args))
				for _, arg := range args {
					callArgs = append(callArgs, processMap[arg])
				}
				callable(callArgs...)
			}
		}
	}
	func24 := func(e float64, t float64, n float64) {
		processMap[e] = turnstileValueToString(processMap[t]) + "." + turnstileValueToString(processMap[n])
	}

	processMap[1] = func1
	processMap[2] = func2
	processMap[3] = func3
	processMap[5] = func5
	processMap[6] = func6
	processMap[7] = func7
	processMap[8] = func8
	processMap[14] = func14
	processMap[15] = func15
	processMap[17] = func17
	processMap[18] = func18
	processMap[19] = func19
	processMap[20] = func20
	processMap[21] = func21
	processMap[23] = func23
	processMap[24] = func24

	for _, token := range tokenList {
		tokenArr, ok := token.([]any)
		if !ok || len(tokenArr) == 0 {
			continue
		}
		op, ok := tokenArr[0].(float64)
		if !ok {
			continue
		}
		switch op {
		case 1:
			if e, ok := openAITurnstileFloatArg(tokenArr, 1); ok {
				if t, ok := openAITurnstileFloatArg(tokenArr, 2); ok {
					func1(e, t)
				}
			}
		case 2:
			if e, ok := openAITurnstileFloatArg(tokenArr, 1); ok && len(tokenArr) >= 3 {
				func2(e, tokenArr[2])
			}
		case 3:
			if len(tokenArr) >= 2 {
				func3(turnstileValueToString(tokenArr[1]))
			}
		case 5:
			if e, ok := openAITurnstileFloatArg(tokenArr, 1); ok {
				if t, ok := openAITurnstileFloatArg(tokenArr, 2); ok {
					func5(e, t)
				}
			}
		case 6:
			if e, ok := openAITurnstileFloatArg(tokenArr, 1); ok {
				if t, ok := openAITurnstileFloatArg(tokenArr, 2); ok {
					if n, ok := openAITurnstileFloatArg(tokenArr, 3); ok {
						func6(e, t, n)
					}
				}
			}
		case 7:
			if e, ok := openAITurnstileFloatArg(tokenArr, 1); ok {
				func7(e, openAITurnstileFloatArgs(tokenArr, 2)...)
			}
		case 8:
			if e, ok := openAITurnstileFloatArg(tokenArr, 1); ok {
				if t, ok := openAITurnstileFloatArg(tokenArr, 2); ok {
					func8(e, t)
				}
			}
		case 14:
			if e, ok := openAITurnstileFloatArg(tokenArr, 1); ok {
				if t, ok := openAITurnstileFloatArg(tokenArr, 2); ok {
					func14(e, t)
				}
			}
		case 15:
			if e, ok := openAITurnstileFloatArg(tokenArr, 1); ok {
				if t, ok := openAITurnstileFloatArg(tokenArr, 2); ok {
					func15(e, t)
				}
			}
		case 17:
			if e, ok := openAITurnstileFloatArg(tokenArr, 1); ok {
				if t, ok := openAITurnstileFloatArg(tokenArr, 2); ok {
					func17(e, t, openAITurnstileFloatArgs(tokenArr, 3)...)
				}
			}
		case 18:
			if e, ok := openAITurnstileFloatArg(tokenArr, 1); ok {
				func18(e)
			}
		case 19:
			if e, ok := openAITurnstileFloatArg(tokenArr, 1); ok {
				func19(e)
			}
		case 20:
			if e, ok := openAITurnstileFloatArg(tokenArr, 1); ok {
				if t, ok := openAITurnstileFloatArg(tokenArr, 2); ok {
					if n, ok := openAITurnstileFloatArg(tokenArr, 3); ok {
						func20(e, t, n, openAITurnstileFloatArgs(tokenArr, 4)...)
					}
				}
			}
		case 21:
			func21()
		case 23:
			if e, ok := openAITurnstileFloatArg(tokenArr, 1); ok {
				if t, ok := openAITurnstileFloatArg(tokenArr, 2); ok {
					func23(e, t, openAITurnstileFloatArgs(tokenArr, 3)...)
				}
			}
		case 24:
			if e, ok := openAITurnstileFloatArg(tokenArr, 1); ok {
				if t, ok := openAITurnstileFloatArg(tokenArr, 2); ok {
					if n, ok := openAITurnstileFloatArg(tokenArr, 3); ok {
						func24(e, t, n)
					}
				}
			}
		}
	}
	return result
}

func openAITurnstileFloatArg(args []any, index int) (float64, bool) {
	if index < 0 || index >= len(args) {
		return 0, false
	}
	v, ok := args[index].(float64)
	return v, ok
}

func openAITurnstileFloatArgs(args []any, start int) []float64 {
	if start < 0 || start >= len(args) {
		return nil
	}
	out := make([]float64, 0, len(args)-start)
	for _, raw := range args[start:] {
		if f, ok := raw.(float64); ok {
			out = append(out, f)
		}
	}
	return out
}

func classifyOpenAIFreeImageSetupError(err error, account *Account) error {
	if err == nil {
		return nil
	}
	msg := sanitizeUpstreamErrorMessage(err.Error())
	lower := strings.ToLower(strings.TrimSpace(msg))
	if strings.Contains(lower, "dial tcp") ||
		strings.Contains(lower, "i/o timeout") ||
		strings.Contains(lower, "tls handshake timeout") ||
		strings.Contains(lower, "timeout awaiting response headers") ||
		strings.Contains(lower, "connectex") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "eof") ||
		strings.Contains(lower, "bootstrap failed: http 5") ||
		strings.Contains(lower, "chat requirements failed: http 5") ||
		strings.Contains(lower, "prepare image conversation failed: http 5") ||
		strings.Contains(lower, "free image upload init failed: http 5") ||
		strings.Contains(lower, "free image upload blob failed: http 5") ||
		strings.Contains(lower, "free image upload confirm failed: http 5") {
		return &UpstreamFailoverError{
			StatusCode:             0,
			ResponseBody:           []byte(msg),
			RetryableOnSameAccount: account != nil && account.IsPoolMode(),
		}
	}
	return err
}

func buildOpenAIImagesFreeConversationRequest(parsed *OpenAIImagesRequest, uploadedRefs []map[string]any) ([]byte, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed images request is required")
	}
	prompt := strings.TrimSpace(parsed.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	inputImages := make([]string, 0, len(parsed.InputImageURLs)+len(parsed.Uploads))
	for _, imageURL := range parsed.InputImageURLs {
		if trimmed := strings.TrimSpace(imageURL); trimmed != "" {
			inputImages = append(inputImages, trimmed)
		}
	}
	for _, upload := range parsed.Uploads {
		dataURL, err := openAIImageUploadToDataURL(upload)
		if err != nil {
			return nil, err
		}
		inputImages = append(inputImages, dataURL)
	}
	if parsed.IsEdits() && len(inputImages) == 0 {
		return nil, fmt.Errorf("image input is required")
	}

	parts := make([]any, 0, len(uploadedRefs)+1)
	for _, ref := range uploadedRefs {
		parts = append(parts, map[string]any{
			"content_type":  "image_asset_pointer",
			"asset_pointer": firstNonEmptyString(ref["asset_ptr"]),
			"width":         ref["width"],
			"height":        ref["height"],
			"size_bytes":    ref["file_size"],
		})
	}
	parts = append(parts, prompt)
	contentType := "text"
	if len(uploadedRefs) > 0 {
		contentType = "multimodal_text"
	}
	metadata := map[string]any{
		"developer_mode_connector_ids": []any{},
		"selected_github_repos":        []any{},
		"selected_all_github_repos":    false,
		"system_hints":                 []any{"picture_v2"},
		"serialization_metadata": map[string]any{
			"custom_symbol_offsets": []any{},
		},
	}
	if len(uploadedRefs) > 0 {
		attachments := make([]any, 0, len(uploadedRefs))
		for _, ref := range uploadedRefs {
			attachments = append(attachments, map[string]any{
				"id":       firstNonEmptyString(ref["file_id"]),
				"mimeType": firstNonEmptyString(ref["mime_type"]),
				"name":     firstNonEmptyString(ref["file_name"]),
				"size":     ref["file_size"],
				"width":    ref["width"],
				"height":   ref["height"],
			})
		}
		metadata["attachments"] = attachments
	}
	payload := map[string]any{
		"action": "next",
		"messages": []any{
			map[string]any{
				"id":          fmt.Sprintf("msg_%d", time.Now().UnixNano()),
				"author":      map[string]any{"role": "user"},
				"create_time": float64(time.Now().UnixNano()) / float64(time.Second),
				"content": map[string]any{
					"content_type": contentType,
					"parts":        parts,
				},
				"metadata": metadata,
			},
		},
		"parent_message_id":        fmt.Sprintf("msg_%d", time.Now().UnixNano()+1),
		"model":                    openAIFreeImageConversationModel,
		"client_prepare_state":     "sent",
		"timezone_offset_min":      -480,
		"timezone":                 "America/Los_Angeles",
		"conversation_mode":        map[string]any{"kind": "primary_assistant"},
		"enable_message_followups": true,
		"system_hints":             []any{"picture_v2"},
		"supports_buffering":       true,
		"supported_encodings":      []any{"v1"},
		"client_contextual_info": map[string]any{
			"is_dark_mode":      false,
			"time_since_loaded": 1200,
			"page_height":       1072,
			"page_width":        1724,
			"pixel_ratio":       1.2,
			"screen_height":     1440,
			"screen_width":      2560,
			"app_name":          "chatgpt.com",
		},
		"paragen_cot_summary_display_override": "allow",
		"force_parallel_switch":                "auto",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal free image conversation payload: %w", err)
	}
	return body, nil
}

func uploadOpenAIFreeImage(
	ctx context.Context,
	client *req.Client,
	account *Account,
	dataURL string,
	fileName string,
) (map[string]any, error) {
	data := dataURL
	if idx := strings.Index(data, ","); idx >= 0 && idx+1 < len(data) {
		data = data[idx+1:]
	}
	imageBytes, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("decode free image upload data: %w", err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, fmt.Errorf("decode free image upload config: %w", err)
	}
	mimeType := "image/png"
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "jpeg", "jpg":
		mimeType = "image/jpeg"
	case "gif":
		mimeType = "image/gif"
	}

	headers := buildOpenAIFreeImageHeaders(account, nil, "", "application/json", "/backend-api/files")
	headers.Set("X-OpenAI-Target-Path", "/backend-api/files")
	headers.Set("X-OpenAI-Target-Route", "/backend-api/files")
	body := map[string]any{
		"file_name": fileName,
		"file_size": len(imageBytes),
		"use_case":  "multimodal",
		"width":     cfg.Width,
		"height":    cfg.Height,
	}
	var uploadMeta map[string]any
	resp, err := client.R().
		SetContext(ctx).
		SetHeaders(headerToMap(headers)).
		SetBodyJsonMarshal(body).
		SetSuccessResult(&uploadMeta).
		Post(openAIChatGPTFilesURL)
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("free image upload init failed: HTTP %d", resp.StatusCode)
	}
	uploadURL := firstNonEmptyString(uploadMeta["upload_url"])
	if uploadURL == "" {
		return nil, fmt.Errorf("free image upload init missing upload_url")
	}
	putResp, err := client.R().
		SetContext(ctx).
		SetHeader("Content-Type", mimeType).
		SetHeader("x-ms-blob-type", "BlockBlob").
		SetHeader("x-ms-version", "2020-04-08").
		SetHeader("Origin", "https://chatgpt.com").
		SetHeader("Referer", "https://chatgpt.com/").
		SetHeader("User-Agent", firstNonEmptyString(account.GetOpenAIUserAgent(), openAIImageBackendUserAgent)).
		SetBody(imageBytes).
		Put(uploadURL)
	if err != nil {
		return nil, err
	}
	if !putResp.IsSuccessState() {
		return nil, fmt.Errorf("free image upload blob failed: HTTP %d", putResp.StatusCode)
	}
	fileID := firstNonEmptyString(uploadMeta["file_id"])
	if fileID == "" {
		return nil, fmt.Errorf("free image upload missing file_id")
	}
	confirmURL := fmt.Sprintf("%s/%s/uploaded", openAIChatGPTFilesURL, fileID)
	confirmResp, err := client.R().
		SetContext(ctx).
		SetHeaders(headerToMap(headers)).
		SetBodyString("{}").
		Post(confirmURL)
	if err != nil {
		return nil, err
	}
	if !confirmResp.IsSuccessState() {
		return nil, fmt.Errorf("free image upload confirm failed: HTTP %d", confirmResp.StatusCode)
	}

	return map[string]any{
		"file_id":   fileID,
		"file_name": fileName,
		"file_size": len(imageBytes),
		"mime_type": mimeType,
		"width":     cfg.Width,
		"height":    cfg.Height,
		"asset_ptr": "file-service://" + fileID,
	}, nil
}

func createOpenAIFreeImageClient(proxyURL string) (*req.Client, error) {
	client := req.C().SetTimeout(30 * time.Second).ImpersonateChrome()
	trimmed := strings.TrimSpace(proxyURL)
	if trimmed == "" {
		trimmed = strings.TrimSpace(os.Getenv("SUB2API_FREE_IMAGE_PROXY_URL"))
	}
	if trimmed != "" {
		client.SetProxyURL(trimmed)
	}
	return client, nil
}

func buildOpenAIFreeImageFingerprint(account *Account) map[string]string {
	fp := map[string]string{
		"user-agent":                  openAIImageBackendUserAgent,
		"sec-ch-ua":                   `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`,
		"sec-ch-ua-mobile":            "?0",
		"sec-ch-ua-platform":          `"Windows"`,
		"oai-device-id":               uuid.NewString(),
		"oai-session-id":              uuid.NewString(),
		"oai-client-version":          openAIDefaultClientVersion,
		"oai-client-build-number":     openAIDefaultClientBuildNumber,
		"sec-ch-ua-arch":              `"x86"`,
		"sec-ch-ua-bitness":           `"64"`,
		"sec-ch-ua-full-version":      `"143.0.3650.96"`,
		"sec-ch-ua-full-version-list": `"Microsoft Edge";v="143.0.3650.96", "Chromium";v="143.0.7499.147", "Not A(Brand";v="24.0.0.0"`,
		"sec-ch-ua-model":             `""`,
		"sec-ch-ua-platform-version":  `"19.0.0"`,
	}
	if account == nil {
		return fp
	}
	if ua := strings.TrimSpace(account.GetOpenAIUserAgent()); ua != "" {
		fp["user-agent"] = ua
	}
	if deviceID := strings.TrimSpace(account.GetOpenAIDeviceID()); deviceID != "" {
		fp["oai-device-id"] = deviceID
	}
	if sessionID := strings.TrimSpace(account.GetOpenAISessionID()); sessionID != "" {
		fp["oai-session-id"] = sessionID
	}
	return fp
}

func buildOpenAIFreeImageHeaders(account *Account, requirements *openAIChatRequirements, conduitToken string, accept string, targetPath string) http.Header {
	fp := buildOpenAIFreeImageFingerprint(account)
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+account.GetOpenAIAccessToken())
	headers.Set("Origin", "https://chatgpt.com")
	headers.Set("Referer", "https://chatgpt.com/")
	headers.Set("User-Agent", fp["user-agent"])
	headers.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Pragma", "no-cache")
	headers.Set("Priority", "u=1, i")
	headers.Set("Sec-Ch-Ua", fp["sec-ch-ua"])
	headers.Set("Sec-Ch-Ua-Arch", fp["sec-ch-ua-arch"])
	headers.Set("Sec-Ch-Ua-Bitness", fp["sec-ch-ua-bitness"])
	headers.Set("Sec-Ch-Ua-Full-Version", fp["sec-ch-ua-full-version"])
	headers.Set("Sec-Ch-Ua-Full-Version-List", fp["sec-ch-ua-full-version-list"])
	headers.Set("Sec-Ch-Ua-Mobile", fp["sec-ch-ua-mobile"])
	headers.Set("Sec-Ch-Ua-Model", fp["sec-ch-ua-model"])
	headers.Set("Sec-Ch-Ua-Platform", fp["sec-ch-ua-platform"])
	headers.Set("Sec-Ch-Ua-Platform-Version", fp["sec-ch-ua-platform-version"])
	headers.Set("Sec-Fetch-Dest", "empty")
	headers.Set("Sec-Fetch-Mode", "cors")
	headers.Set("Sec-Fetch-Site", "same-origin")
	headers.Set("OAI-Language", "zh-CN")
	headers.Set("OAI-Client-Version", fp["oai-client-version"])
	headers.Set("OAI-Client-Build-Number", fp["oai-client-build-number"])
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", accept)
	if account.GetChatGPTAccountID() != "" {
		headers.Set("chatgpt-account-id", account.GetChatGPTAccountID())
	}
	headers.Set("OAI-Device-Id", fp["oai-device-id"])
	headers.Set("OAI-Session-Id", fp["oai-session-id"])
	if requirements != nil {
		if strings.TrimSpace(requirements.Token) != "" {
			headers.Set("OpenAI-Sentinel-Chat-Requirements-Token", strings.TrimSpace(requirements.Token))
		}
		if strings.TrimSpace(requirements.ProofToken) != "" {
			headers.Set("OpenAI-Sentinel-Proof-Token", strings.TrimSpace(requirements.ProofToken))
		}
		if strings.TrimSpace(requirements.TurnstileToken) != "" {
			headers.Set("OpenAI-Sentinel-Turnstile-Token", strings.TrimSpace(requirements.TurnstileToken))
		}
	}
	if strings.TrimSpace(conduitToken) != "" {
		headers.Set("X-Conduit-Token", strings.TrimSpace(conduitToken))
	}
	if strings.EqualFold(strings.TrimSpace(accept), "text/event-stream") {
		headers.Set("X-Oai-Turn-Trace-Id", uuid.NewString())
	}
	route := strings.TrimSpace(targetPath)
	if route == "" {
		route = "/backend-api/f/conversation"
	}
	headers.Set("X-OpenAI-Target-Path", route)
	headers.Set("X-OpenAI-Target-Route", route)
	return headers
}

func fetchOpenAIFreeImageRequirements(
	ctx context.Context,
	client *req.Client,
	account *Account,
) (*openAIChatRequirements, error) {
	if client == nil {
		return nil, fmt.Errorf("free image client is required")
	}

	bootstrapResp, err := client.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+account.GetOpenAIAccessToken()).
		SetHeader("User-Agent", firstNonEmptyString(account.GetOpenAIUserAgent(), openAIImageBackendUserAgent)).
		SetHeader("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8").
		Get(openAIChatGPTStartURL)
	if err != nil {
		return nil, err
	}
	if !bootstrapResp.IsSuccessState() {
		return nil, fmt.Errorf("bootstrap failed: HTTP %d", bootstrapResp.StatusCode)
	}
	scriptSources, dataBuild := parseOpenAIPowResources(bootstrapResp.String())
	reqBody := map[string]any{
		"p": buildOpenAILegacyRequirementsToken(firstNonEmptyString(account.GetOpenAIUserAgent(), openAIImageBackendUserAgent), scriptSources, dataBuild),
	}
	var payload map[string]any
	resp, err := client.R().
		SetContext(ctx).
		SetHeaders(headerToMap(buildOpenAIFreeImageHeaders(account, nil, "", "application/json", "/backend-api/sentinel/chat-requirements"))).
		SetBodyJsonMarshal(reqBody).
		SetSuccessResult(&payload).
		Post(openAIChatGPTRequirementsURL)
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("chat requirements failed: HTTP %d", resp.StatusCode)
	}
	requirements := &openAIChatRequirements{
		Token: strings.TrimSpace(firstNonEmptyString(payload["token"])),
	}
	if requirements.Token == "" {
		return nil, fmt.Errorf("missing chat requirements token")
	}
	if proofInfo, ok := payload["proofofwork"].(map[string]any); ok {
		required, _ := proofInfo["required"].(bool)
		if required {
			proofToken, proofErr := buildOpenAIProofToken(
				firstNonEmptyString(proofInfo["seed"]),
				firstNonEmptyString(proofInfo["difficulty"]),
				firstNonEmptyString(account.GetOpenAIUserAgent(), openAIImageBackendUserAgent),
				scriptSources,
				dataBuild,
			)
			if proofErr != nil {
				return nil, proofErr
			}
			requirements.ProofToken = proofToken
		}
	}
	if turnstileInfo, ok := payload["turnstile"].(map[string]any); ok {
		required, _ := turnstileInfo["required"].(bool)
		if required {
			dx := firstNonEmptyString(turnstileInfo["dx"])
			if dx != "" {
				requirements.TurnstileToken = solveOpenAITurnstileToken(dx, firstNonEmptyString(reqBody["p"]))
			}
		}
	}
	return requirements, nil
}

func prepareOpenAIFreeImageConversation(
	ctx context.Context,
	client *req.Client,
	account *Account,
	requirements *openAIChatRequirements,
	prompt string,
) (string, error) {
	preparePayload := map[string]any{
		"action":                "next",
		"fork_from_shared_post": false,
		"parent_message_id":     uuid.NewString(),
		"model":                 openAIFreeImageConversationModel,
		"client_prepare_state":  "success",
		"timezone_offset_min":   -480,
		"timezone":              "America/Los_Angeles",
		"conversation_mode":     map[string]any{"kind": "primary_assistant"},
		"system_hints":          []any{"picture_v2"},
		"partial_query": map[string]any{
			"id":      uuid.NewString(),
			"author":  map[string]any{"role": "user"},
			"content": map[string]any{"content_type": "text", "parts": []any{prompt}},
		},
		"supports_buffering":  true,
		"supported_encodings": []any{"v1"},
		"client_contextual_info": map[string]any{
			"app_name": "chatgpt.com",
		},
	}
	var result map[string]any
	resp, err := client.R().
		SetContext(ctx).
		SetHeaders(headerToMap(buildOpenAIFreeImageHeaders(account, requirements, "", "application/json", "/backend-api/f/conversation/prepare"))).
		SetBodyJsonMarshal(preparePayload).
		SetSuccessResult(&result).
		Post(openAIChatGPTPrepareURL)
	if err != nil {
		return "", err
	}
	if !resp.IsSuccessState() {
		return "", fmt.Errorf("prepare image conversation failed: HTTP %d", resp.StatusCode)
	}
	return strings.TrimSpace(firstNonEmptyString(result["conduit_token"])), nil
}

func addUniqueStrings(dst []string, values ...string) []string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		exists := false
		for _, existing := range dst {
			if existing == trimmed {
				exists = true
				break
			}
		}
		if !exists {
			dst = append(dst, trimmed)
		}
	}
	return dst
}

func extractOpenAIFreeConversationIDs(payload string) (string, []string, []string) {
	conversationID := strings.TrimSpace(gjson.Get(payload, "conversation_id").String())
	if conversationID == "" {
		conversationID = strings.TrimSpace(gjson.Get(payload, "v.conversation_id").String())
	}
	fileIDs := dedupeStrings(regexp.MustCompile(`(file[-_][A-Za-z0-9]+)`).FindAllString(payload, -1))
	sedimentIDs := extractOpenAIFreeConversationRegexSubmatches(payload, `sediment://([A-Za-z0-9_-]+)`)
	return conversationID, fileIDs, sedimentIDs
}

func extractOpenAIFreeConversationRegexSubmatches(payload string, pattern string) []string {
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(payload, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			values = append(values, match[1])
		}
	}
	return dedupeStrings(values)
}

func openAIFreeImageAssistantMessageText(message map[string]any) string {
	content, _ := message["content"].(map[string]any)
	parts, _ := content["parts"].([]any)
	if len(parts) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, part := range parts {
		if text, ok := part.(string); ok {
			_, _ = builder.WriteString(text)
		}
	}
	return strings.TrimSpace(builder.String())
}

func isOpenAIFreeImageToolEvent(event map[string]any) bool {
	value, _ := event["v"].(map[string]any)
	message, _ := event["message"].(map[string]any)
	if message == nil && value != nil {
		message, _ = value["message"].(map[string]any)
	}
	if message == nil {
		return false
	}
	author, _ := message["author"].(map[string]any)
	metadata, _ := message["metadata"].(map[string]any)
	return strings.TrimSpace(firstNonEmptyString(author["role"])) == "tool" &&
		strings.TrimSpace(firstNonEmptyString(metadata["async_task_type"])) == "image_gen"
}

func updateOpenAIFreeImageConversationState(state *openAIFreeImageConversationState, payload string, event map[string]any) {
	if state == nil {
		return
	}
	conversationID, fileIDs, sedimentIDs := extractOpenAIFreeConversationIDs(payload)
	if state.ConversationID == "" && conversationID != "" {
		state.ConversationID = conversationID
	}
	if isOpenAIFreeImageToolEvent(event) {
		state.FileIDs = addUniqueStrings(state.FileIDs, fileIDs...)
		state.SedimentIDs = addUniqueStrings(state.SedimentIDs, sedimentIDs...)
	}
	if value, ok := event["v"].(map[string]any); ok {
		if convID := strings.TrimSpace(firstNonEmptyString(value["conversation_id"])); convID != "" {
			state.ConversationID = convID
		}
		if message, ok := value["message"].(map[string]any); ok {
			if author, ok := message["author"].(map[string]any); ok && strings.TrimSpace(firstNonEmptyString(author["role"])) == "assistant" {
				if text := openAIFreeImageAssistantMessageText(message); text != "" {
					state.Text = text
				}
			}
		}
	}
	if moderation, ok := event["moderation_response"].(map[string]any); ok {
		if blocked, ok := moderation["blocked"].(bool); ok && blocked {
			state.Blocked = true
		}
	}
	if metadata, ok := event["metadata"].(map[string]any); ok {
		if toolInvoked, ok := metadata["tool_invoked"].(bool); ok {
			state.ToolInvoked = &toolInvoked
		}
		if turnUseCase := strings.TrimSpace(firstNonEmptyString(metadata["turn_use_case"])); turnUseCase != "" {
			state.TurnUseCase = turnUseCase
		}
	}
	if message, ok := event["message"].(map[string]any); ok {
		if author, ok := message["author"].(map[string]any); ok && strings.TrimSpace(firstNonEmptyString(author["role"])) == "assistant" {
			if text := openAIFreeImageAssistantMessageText(message); text != "" {
				state.Text = text
			}
		}
	}
}

func readOpenAIFreeImageConversationStream(respBody io.Reader) ([]byte, *openAIFreeImageConversationState, error) {
	if respBody == nil {
		return nil, nil, fmt.Errorf("free image conversation body is required")
	}
	var (
		rawBody bytes.Buffer
		state   = &openAIFreeImageConversationState{}
		acc     openAISSEDataAccumulator
	)
	reader := bufio.NewReader(respBody)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			_, _ = rawBody.WriteString(line)
			acc.AddLine(strings.TrimRight(line, "\r\n"), func(data []byte) {
				if !gjson.ValidBytes(data) {
					return
				}
				var event map[string]any
				if json.Unmarshal(data, &event) != nil {
					return
				}
				updateOpenAIFreeImageConversationState(state, string(data), event)
			})
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
	}
	acc.Flush(func(data []byte) {
		if !gjson.ValidBytes(data) {
			return
		}
		var event map[string]any
		if json.Unmarshal(data, &event) != nil {
			return
		}
		updateOpenAIFreeImageConversationState(state, string(data), event)
	})
	return rawBody.Bytes(), state, nil
}

func startOpenAIFreeImageConversation(
	ctx context.Context,
	client *req.Client,
	account *Account,
	requirements *openAIChatRequirements,
	conduitToken string,
	body []byte,
) ([]byte, *openAIFreeImageConversationState, error) {
	if client == nil {
		return nil, nil, fmt.Errorf("free image client is required")
	}
	headers := buildOpenAIFreeImageHeaders(account, requirements, conduitToken, "text/event-stream", "/backend-api/f/conversation")
	resp, err := client.R().
		SetContext(ctx).
		SetHeaders(headerToMap(headers)).
		SetBody(body).
		DisableAutoReadResponse().
		Post(openAIChatGPTConversationURL)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if !resp.IsSuccessState() {
		return nil, nil, newOpenAIImageStatusError(resp, "free image conversation failed")
	}
	return readOpenAIFreeImageConversationStream(resp.Body)
}

func (s *OpenAIGatewayService) openAIFreeImagePollTimeout() time.Duration {
	if s == nil || s.cfg == nil || s.cfg.Gateway.ImageStreamDataIntervalTimeout <= 0 {
		return 120 * time.Second
	}
	return time.Duration(s.cfg.Gateway.ImageStreamDataIntervalTimeout) * time.Second
}

func (s *OpenAIGatewayService) pollOpenAIFreeImageResultURLs(
	ctx context.Context,
	account *Account,
	state *openAIFreeImageConversationState,
) ([]string, error) {
	if state == nil {
		return nil, fmt.Errorf("free image conversation state is required")
	}
	client := req.C().SetTimeout(30 * time.Second).ImpersonateChrome()
	headers := http.Header{
		"Authorization":      []string{"Bearer " + account.GetOpenAIAccessToken()},
		"Accept":             []string{"application/json"},
		"Origin":             []string{"https://chatgpt.com"},
		"Referer":            []string{"https://chatgpt.com/"},
		"User-Agent":         []string{firstNonEmptyString(account.GetOpenAIUserAgent(), openAIImageBackendUserAgent)},
		"chatgpt-account-id": []string{account.GetChatGPTAccountID()},
	}
	if deviceID := strings.TrimSpace(account.GetOpenAIDeviceID()); deviceID != "" {
		headers.Set("OAI-Device-Id", deviceID)
	}
	if sessionID := strings.TrimSpace(account.GetOpenAISessionID()); sessionID != "" {
		headers.Set("OAI-Session-Id", sessionID)
	}

	resolve := func() ([]string, error) {
		var urls []string
		for _, fileID := range state.FileIDs {
			if strings.TrimSpace(fileID) == "" || fileID == "file_upload" {
				continue
			}
			downloadURL, err := fetchOpenAIImageDownloadURL(ctx, client, headers, state.ConversationID, "file-service://"+fileID)
			if err != nil {
				continue
			}
			if strings.TrimSpace(downloadURL) != "" {
				urls = append(urls, strings.TrimSpace(downloadURL))
			}
		}
		if len(urls) > 0 {
			return dedupeStrings(urls), nil
		}
		for _, sedimentID := range state.SedimentIDs {
			if strings.TrimSpace(sedimentID) == "" {
				continue
			}
			downloadURL, err := fetchOpenAIImageDownloadURL(ctx, client, headers, state.ConversationID, "sediment://"+sedimentID)
			if err != nil {
				continue
			}
			if strings.TrimSpace(downloadURL) != "" {
				urls = append(urls, strings.TrimSpace(downloadURL))
			}
		}
		if len(urls) > 0 {
			return dedupeStrings(urls), nil
		}
		if strings.TrimSpace(state.ConversationID) == "" {
			return nil, nil
		}
		conversationURL := fmt.Sprintf("https://chatgpt.com/backend-api/conversation/%s", state.ConversationID)
		var payload map[string]any
		resp, err := client.R().
			SetContext(ctx).
			SetHeaders(headerToMap(headers)).
			SetSuccessResult(&payload).
			Get(conversationURL)
		if err != nil {
			return nil, err
		}
		if !resp.IsSuccessState() {
			return nil, fmt.Errorf("free image conversation poll failed: HTTP %d", resp.StatusCode)
		}
		raw, _ := json.Marshal(payload)
		if len(raw) > 0 {
			var event map[string]any
			if value, ok := payload["mapping"].(map[string]any); ok {
				event = map[string]any{"v": map[string]any{"conversation_id": state.ConversationID, "mapping": value}}
			}
			updateOpenAIFreeImageConversationState(state, string(raw), event)
		}
		return nil, nil
	}

	deadline := time.Now().Add(s.openAIFreeImagePollTimeout())
	for {
		urls, err := resolve()
		if err != nil {
			return nil, err
		}
		if len(urls) > 0 {
			return urls, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("free image conversation poll timeout")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(4 * time.Second):
		}
	}

}

func rewriteOpenAIImagesModel(body []byte, contentType string, model string) ([]byte, string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return body, contentType, nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		rewrittenBody, rewrittenType, rewriteErr := rewriteOpenAIImagesMultipartModel(body, contentType, model)
		return rewrittenBody, rewrittenType, rewriteErr
	}
	rewritten, err := sjson.SetBytes(body, "model", model)
	if err != nil {
		return nil, "", fmt.Errorf("rewrite image request model: %w", err)
	}
	return rewritten, contentType, nil
}

func (s *OpenAIGatewayService) handleOpenAIImagesOAuthFreeConversationBody(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	state *openAIFreeImageConversationState,
	imageURLs []string,
	parsed *OpenAIImagesRequest,
) ([]byte, OpenAIUsage, error) {
	usage := OpenAIUsage{}
	mergeOpenAIUsage(&usage, body)
	if len(imageURLs) == 0 {
		return nil, usage, fmt.Errorf("free image conversation returned no image urls")
	}
	client := req.C().SetTimeout(30 * time.Second).ImpersonateChrome()
	data := make([]map[string]any, 0, len(imageURLs))
	for _, imageURL := range imageURLs {
		imgBytes, err := downloadOpenAIImageBytes(ctx, client, http.Header{
			"Authorization":      []string{"Bearer " + account.GetOpenAIAccessToken()},
			"Accept":             []string{"application/json"},
			"Origin":             []string{"https://chatgpt.com"},
			"Referer":            []string{"https://chatgpt.com/"},
			"User-Agent":         []string{firstNonEmptyString(account.GetOpenAIUserAgent(), openAIImageBackendUserAgent)},
			"chatgpt-account-id": []string{account.GetChatGPTAccountID()},
		}, imageURL)
		if err != nil {
			return nil, usage, err
		}
		entry := map[string]any{
			"revised_prompt": strings.TrimSpace(firstNonEmptyString(gjson.GetBytes(body, "message.metadata.dalle.prompt").String(), state.Text, parsed.Prompt)),
		}
		if strings.EqualFold(strings.TrimSpace(parsed.ResponseFormat), "url") {
			entry["url"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(imgBytes)
		} else {
			entry["b64_json"] = base64.StdEncoding.EncodeToString(imgBytes)
		}
		data = append(data, entry)
	}

	result := map[string]any{
		"created": time.Now().Unix(),
		"data":    data,
	}
	out, err := json.Marshal(result)
	if err != nil {
		return nil, usage, fmt.Errorf("marshal free image result: %w", err)
	}
	return out, usage, nil
}

func rewriteOpenAIImagesMultipartModel(body []byte, contentType string, model string) ([]byte, string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", fmt.Errorf("parse multipart content-type: %w", err)
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, "", fmt.Errorf("multipart boundary is required")
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	modelWritten := false

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("read multipart body: %w", err)
		}

		formName := strings.TrimSpace(part.FormName())
		partHeader := cloneMultipartHeader(part.Header)
		target, err := writer.CreatePart(partHeader)
		if err != nil {
			_ = part.Close()
			return nil, "", fmt.Errorf("create multipart part: %w", err)
		}

		if formName == "model" && part.FileName() == "" {
			if _, err := target.Write([]byte(model)); err != nil {
				_ = part.Close()
				return nil, "", fmt.Errorf("rewrite multipart model: %w", err)
			}
			modelWritten = true
			_ = part.Close()
			continue
		}
		if _, err := io.Copy(target, part); err != nil {
			_ = part.Close()
			return nil, "", fmt.Errorf("copy multipart part: %w", err)
		}
		_ = part.Close()
	}

	if !modelWritten {
		if err := writer.WriteField("model", model); err != nil {
			return nil, "", fmt.Errorf("append multipart model field: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("finalize multipart body: %w", err)
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func cloneMultipartHeader(src textproto.MIMEHeader) textproto.MIMEHeader {
	dst := make(textproto.MIMEHeader, len(src))
	for key, values := range src {
		copied := make([]string, len(values))
		copy(copied, values)
		dst[key] = copied
	}
	return dst
}

func (s *OpenAIGatewayService) handleOpenAIImagesNonStreamingResponse(resp *http.Response, c *gin.Context) (OpenAIUsage, int, []string, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return OpenAIUsage{}, 0, nil, err
	}
	body = enrichOpenAIImagesAPIResponseSizes(body)
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
			contentType = upstreamType
		}
	}
	c.Data(resp.StatusCode, contentType, body)

	usage, _ := extractOpenAIUsageFromJSONBytes(body)
	return usage, extractOpenAIImageCountFromJSONBytes(body), collectOpenAIResponseImageOutputSizesFromJSONBytes(body), nil
}

func (s *OpenAIGatewayService) handleOpenAIImagesStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	startTime time.Time,
) (OpenAIUsage, int, []string, *int, error) {
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "text/event-stream"
	}
	c.Status(resp.StatusCode)
	c.Header("Content-Type", contentType)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return OpenAIUsage{}, 0, nil, nil, fmt.Errorf("streaming is not supported by response writer")
	}

	usage := OpenAIUsage{}
	imageCounter := newOpenAIImageOutputCounter()
	var firstTokenMs *int
	clientDisconnected := false
	lastDownstreamWriteAt := time.Now()
	var fallbackBody bytes.Buffer
	fallbackBytes := int64(0)
	fallbackLimit := resolveUpstreamResponseReadLimit(s.cfg)
	seenSSEData := false
	fallbackTooLarge := false
	var sseData openAISSEDataAccumulator

	processSSEData := func(dataBytes []byte) {
		seenSSEData = true
		fallbackBody.Reset()
		fallbackBytes = 0
		mergeOpenAIUsage(&usage, dataBytes)
		imageCounter.AddSSEData(dataBytes)
	}

	flushSSEEvent := func() {
		sseData.Flush(processSSEData)
	}

	processLine := func(line []byte) {
		if len(line) == 0 {
			return
		}
		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		if !clientDisconnected {
			if _, writeErr := c.Writer.Write(line); writeErr != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images stream client disconnected, continue draining upstream for billing")
			} else {
				flusher.Flush()
				lastDownstreamWriteAt = time.Now()
			}
		}

		trimmedLine := strings.TrimRight(string(line), "\r\n")
		if _, ok := extractOpenAISSEDataLine(trimmedLine); ok || strings.TrimSpace(trimmedLine) == "" {
			sseData.AddLine(trimmedLine, processSSEData)
			return
		}
		if !seenSSEData && !fallbackTooLarge {
			fallbackBytes += int64(len(line))
			if fallbackBytes <= fallbackLimit {
				_, _ = fallbackBody.Write(line)
			} else {
				fallbackTooLarge = true
				fallbackBody.Reset()
			}
		}
	}

	finalizeFallbackBody := func() {
		if seenSSEData || fallbackBody.Len() == 0 {
			return
		}
		body := bytes.TrimSpace(fallbackBody.Bytes())
		if len(body) == 0 {
			return
		}
		mergeOpenAIUsage(&usage, body)
		imageCounter.AddJSONResponse(body)
	}

	streamInterval := s.openAIImageStreamDataInterval()
	keepaliveInterval := s.openAIImageStreamKeepaliveInterval()
	if streamInterval <= 0 && keepaliveInterval <= 0 {
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			processLine(line)
			if err == io.EOF {
				break
			}
			if err != nil {
				flushSSEEvent()
				return usage, imageCounter.Count(), imageCounter.Sizes(), firstTokenMs, err
			}
		}
		flushSSEEvent()
		finalizeFallbackBody()
		return usage, imageCounter.Count(), imageCounter.Sizes(), firstTokenMs, nil
	}

	type readEvent struct {
		line []byte
		err  error
	}
	events := make(chan readEvent, 16)
	done := make(chan struct{})
	sendEvent := func(ev readEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
		}
	}
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func() {
		defer close(events)
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			}
			if len(line) > 0 && !sendEvent(readEvent{line: line}) {
				return
			}
			if err == io.EOF {
				return
			}
			if err != nil {
				_ = sendEvent(readEvent{err: err})
				return
			}
		}
	}()
	defer close(done)

	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
	}
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
	}

	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
	}
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
	}

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				flushSSEEvent()
				finalizeFallbackBody()
				return usage, imageCounter.Count(), imageCounter.Sizes(), firstTokenMs, nil
			}
			if ev.err != nil {
				flushSSEEvent()
				return usage, imageCounter.Count(), imageCounter.Sizes(), firstTokenMs, ev.err
			}
			processLine(ev.line)
		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			if clientDisconnected {
				return usage, imageCounter.Count(), imageCounter.Sizes(), firstTokenMs, fmt.Errorf("image stream incomplete after timeout")
			}
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images stream data interval timeout: interval=%s", streamInterval)
			_ = s.writeOpenAIImagesStreamEvent(c, flusher, "error", buildOpenAIImagesStreamErrorBody(fmt.Sprintf("upstream image stream idle for %s", streamInterval)))
			return usage, imageCounter.Count(), imageCounter.Sizes(), firstTokenMs, fmt.Errorf("image stream data interval timeout")
		case <-keepaliveCh:
			if clientDisconnected || time.Since(lastDownstreamWriteAt) < keepaliveInterval {
				continue
			}
			if _, writeErr := io.WriteString(c.Writer, ":\n\n"); writeErr != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images stream client disconnected during keepalive, continue draining upstream for billing")
				continue
			}
			flusher.Flush()
			lastDownstreamWriteAt = time.Now()
		}
	}
}

func (s *OpenAIGatewayService) openAIImageStreamDataInterval() time.Duration {
	if s == nil || s.cfg == nil || s.cfg.Gateway.ImageStreamDataIntervalTimeout <= 0 {
		return 0
	}
	return time.Duration(s.cfg.Gateway.ImageStreamDataIntervalTimeout) * time.Second
}

func (s *OpenAIGatewayService) openAIImageStreamKeepaliveInterval() time.Duration {
	if s == nil || s.cfg == nil || s.cfg.Gateway.ImageStreamKeepaliveInterval <= 0 {
		return 0
	}
	return time.Duration(s.cfg.Gateway.ImageStreamKeepaliveInterval) * time.Second
}

func extractOpenAIImagesBillableCountFromJSONBytes(body []byte) int {
	if count := extractOpenAIImageCountFromJSONBytes(body); count > 0 {
		return count
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return 0
	}
	if count := int(gjson.GetBytes(body, "usage.images").Int()); count > 0 {
		return count
	}
	if count := int(gjson.GetBytes(body, "tool_usage.image_gen.images").Int()); count > 0 {
		return count
	}
	eventType := strings.TrimSpace(gjson.GetBytes(body, "type").String())
	if eventType == "" || !strings.HasSuffix(eventType, ".completed") {
		return 0
	}
	if gjson.GetBytes(body, "b64_json").Exists() || gjson.GetBytes(body, "url").Exists() {
		return 1
	}
	return 0
}

func mergeOpenAIUsage(dst *OpenAIUsage, body []byte) {
	if dst == nil {
		return
	}
	if parsed, ok := extractOpenAIUsageFromJSONBytes(body); ok {
		if parsed.InputTokens > 0 {
			dst.InputTokens = parsed.InputTokens
		}
		if parsed.OutputTokens > 0 {
			dst.OutputTokens = parsed.OutputTokens
		}
		if parsed.CacheReadInputTokens > 0 {
			dst.CacheReadInputTokens = parsed.CacheReadInputTokens
		}
		if parsed.ImageInputTokens > 0 {
			dst.ImageInputTokens = parsed.ImageInputTokens
		}
		if parsed.ImageOutputTokens > 0 {
			dst.ImageOutputTokens = parsed.ImageOutputTokens
		}
	}
}

func extractOpenAIImageCountFromJSONBytes(body []byte) int {
	return countOpenAIResponseImageOutputsFromJSONBytes(body)
}

type openAIImagePointerInfo struct {
	Pointer     string
	DownloadURL string
	B64JSON     string
	MimeType    string
	Prompt      string
}

func collectOpenAIImagePointers(body []byte) []openAIImagePointerInfo {
	if len(body) == 0 {
		return nil
	}
	prompt := ""
	for _, path := range []string{
		"message.metadata.dalle.prompt",
		"metadata.dalle.prompt",
		"revised_prompt",
	} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			prompt = value
			break
		}
	}
	matches := openAIImagePointerMatches(body)
	out := make([]openAIImagePointerInfo, 0, len(matches))
	for _, pointer := range matches {
		out = append(out, openAIImagePointerInfo{Pointer: pointer, Prompt: prompt})
	}
	return mergeOpenAIImagePointerInfos(out, collectOpenAIImageInlineAssets(body, prompt))
}

func openAIImagePointerMatches(body []byte) []string {
	raw := string(body)
	matches := make([]string, 0, 4)
	for _, prefix := range []string{"file-service://", "sediment://"} {
		start := 0
		for {
			idx := strings.Index(raw[start:], prefix)
			if idx < 0 {
				break
			}
			idx += start
			end := idx + len(prefix)
			for end < len(raw) {
				ch := raw[end]
				if ch != '-' && ch != '_' &&
					(ch < '0' || ch > '9') &&
					(ch < 'a' || ch > 'z') &&
					(ch < 'A' || ch > 'Z') {
					break
				}
				end++
			}
			matches = append(matches, raw[idx:end])
			start = end
		}
	}
	return dedupeStrings(matches)
}

func mergeOpenAIImagePointerInfos(existing []openAIImagePointerInfo, next []openAIImagePointerInfo) []openAIImagePointerInfo {
	if len(next) == 0 {
		return existing
	}
	seen := make(map[string]openAIImagePointerInfo, len(existing)+len(next))
	out := make([]openAIImagePointerInfo, 0, len(existing)+len(next))
	for _, item := range existing {
		if key := item.identityKey(); key != "" {
			seen[key] = item
		}
		out = append(out, item)
	}
	for _, item := range next {
		key := item.identityKey()
		if key == "" {
			continue
		}
		if existingItem, ok := seen[key]; ok {
			merged := mergeOpenAIImagePointerInfo(existingItem, item)
			if merged != existingItem {
				for i := range out {
					if out[i].identityKey() == key {
						out[i] = merged
						break
					}
				}
				seen[key] = merged
			}
			continue
		}
		seen[key] = item
		out = append(out, item)
	}
	return out
}

func (i openAIImagePointerInfo) identityKey() string {
	switch {
	case strings.TrimSpace(i.Pointer) != "":
		return "pointer:" + strings.TrimSpace(i.Pointer)
	case strings.TrimSpace(i.DownloadURL) != "":
		return "download:" + strings.TrimSpace(i.DownloadURL)
	case strings.TrimSpace(i.B64JSON) != "":
		b64 := strings.TrimSpace(i.B64JSON)
		if len(b64) > 64 {
			b64 = b64[:64]
		}
		return "b64:" + b64
	default:
		return ""
	}
}

func mergeOpenAIImagePointerInfo(existing, next openAIImagePointerInfo) openAIImagePointerInfo {
	merged := existing
	if strings.TrimSpace(merged.Pointer) == "" {
		merged.Pointer = next.Pointer
	}
	if strings.TrimSpace(merged.DownloadURL) == "" {
		merged.DownloadURL = next.DownloadURL
	}
	if strings.TrimSpace(merged.B64JSON) == "" {
		merged.B64JSON = next.B64JSON
	}
	if strings.TrimSpace(merged.MimeType) == "" {
		merged.MimeType = next.MimeType
	}
	if strings.TrimSpace(merged.Prompt) == "" {
		merged.Prompt = next.Prompt
	}
	return merged
}

func resolveOpenAIImageBytes(
	ctx context.Context,
	client *req.Client,
	headers http.Header,
	conversationID string,
	pointer openAIImagePointerInfo,
) ([]byte, error) {
	if normalized := normalizeOpenAIImageBase64(pointer.B64JSON); normalized != "" {
		return base64.StdEncoding.DecodeString(normalized)
	}
	if downloadURL := strings.TrimSpace(pointer.DownloadURL); downloadURL != "" {
		return downloadOpenAIImageBytes(ctx, client, headers, downloadURL)
	}
	if strings.TrimSpace(pointer.Pointer) == "" {
		return nil, fmt.Errorf("image asset is missing pointer, url, and base64 data")
	}
	downloadURL, err := fetchOpenAIImageDownloadURL(ctx, client, headers, conversationID, pointer.Pointer)
	if err != nil {
		return nil, err
	}
	return downloadOpenAIImageBytes(ctx, client, headers, downloadURL)
}

func normalizeOpenAIImageBase64(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		if idx := strings.Index(raw, ","); idx >= 0 && idx+1 < len(raw) {
			raw = raw[idx+1:]
		}
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "=") + strings.Repeat("=", (4-len(raw)%4)%4)
	if raw == "" {
		return ""
	}
	if _, err := base64.StdEncoding.DecodeString(raw); err != nil {
		return ""
	}
	return raw
}

func collectOpenAIImageInlineAssets(body []byte, fallbackPrompt string) []openAIImagePointerInfo {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	var out []openAIImagePointerInfo
	walkOpenAIImageInlineAssets(decoded, strings.TrimSpace(fallbackPrompt), &out)
	return out
}

func walkOpenAIImageInlineAssets(node any, prompt string, out *[]openAIImagePointerInfo) {
	switch value := node.(type) {
	case map[string]any:
		localPrompt := prompt
		for _, key := range []string{"revised_prompt", "image_gen_title", "prompt"} {
			if v, ok := value[key].(string); ok && strings.TrimSpace(v) != "" {
				localPrompt = strings.TrimSpace(v)
				break
			}
		}
		item := openAIImagePointerInfo{
			Prompt:      localPrompt,
			Pointer:     firstNonEmptyString(value["asset_pointer"], value["pointer"]),
			DownloadURL: firstNonEmptyString(value["download_url"], value["url"], value["image_url"]),
			B64JSON:     firstNonEmptyString(value["b64_json"], value["base64"], value["image_base64"]),
			MimeType:    firstNonEmptyString(value["mime_type"], value["mimeType"], value["content_type"]),
		}
		switch {
		case strings.HasPrefix(strings.TrimSpace(item.Pointer), "file-service://"),
			strings.HasPrefix(strings.TrimSpace(item.Pointer), "sediment://"),
			isLikelyOpenAIImageDownloadURL(item.DownloadURL),
			normalizeOpenAIImageBase64(item.B64JSON) != "":
			*out = append(*out, item)
		}
		for _, child := range value {
			walkOpenAIImageInlineAssets(child, localPrompt, out)
		}
	case []any:
		for _, child := range value {
			walkOpenAIImageInlineAssets(child, prompt, out)
		}
	}
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func isLikelyOpenAIImageDownloadURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:image/") {
		return true
	}
	if !strings.HasPrefix(strings.ToLower(raw), "http://") && !strings.HasPrefix(strings.ToLower(raw), "https://") {
		return false
	}
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "/download") ||
		strings.Contains(lower, ".png") ||
		strings.Contains(lower, ".jpg") ||
		strings.Contains(lower, ".jpeg") ||
		strings.Contains(lower, ".webp")
}

func fetchOpenAIImageDownloadURL(
	ctx context.Context,
	client *req.Client,
	headers http.Header,
	conversationID string,
	pointer string,
) (string, error) {
	url := ""
	allowConversationRetry := false
	switch {
	case strings.HasPrefix(pointer, "file-service://"):
		fileID := strings.TrimPrefix(pointer, "file-service://")
		url = fmt.Sprintf("%s/%s/download", openAIChatGPTFilesURL, fileID)
	case strings.HasPrefix(pointer, "sediment://"):
		attachmentID := strings.TrimPrefix(pointer, "sediment://")
		url = fmt.Sprintf("https://chatgpt.com/backend-api/conversation/%s/attachment/%s/download", conversationID, attachmentID)
		allowConversationRetry = true
	default:
		return "", fmt.Errorf("unsupported image pointer: %s", pointer)
	}

	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		var result struct {
			DownloadURL string `json:"download_url"`
		}
		resp, err := client.R().
			SetContext(ctx).
			SetHeaders(headerToMap(headers)).
			SetSuccessResult(&result).
			Get(url)
		if err != nil {
			lastErr = err
		} else if resp.IsSuccessState() && strings.TrimSpace(result.DownloadURL) != "" {
			return strings.TrimSpace(result.DownloadURL), nil
		} else {
			statusErr := newOpenAIImageStatusError(resp, "fetch image download url failed")
			if !allowConversationRetry || !isOpenAIImageTransientConversationNotFoundError(statusErr) {
				return "", statusErr
			}
			lastErr = statusErr
		}
		if attempt == 7 {
			break
		}
		timer := time.NewTimer(750 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("fetch image download url failed")
	}
	return "", lastErr
}

func downloadOpenAIImageBytes(ctx context.Context, client *req.Client, headers http.Header, downloadURL string) ([]byte, error) {
	request := client.R().
		SetContext(ctx).
		DisableAutoReadResponse()

	if strings.HasPrefix(downloadURL, openAIChatGPTStartURL) {
		downloadHeaders := cloneHTTPHeader(headers)
		downloadHeaders.Set("Accept", "image/*,*/*;q=0.8")
		downloadHeaders.Del("Content-Type")
		request.SetHeaders(headerToMap(downloadHeaders))
	} else {
		userAgent := strings.TrimSpace(headers.Get("User-Agent"))
		if userAgent == "" {
			userAgent = openAIImageBackendUserAgent
		}
		request.SetHeader("User-Agent", userAgent)
	}

	resp, err := request.Get(downloadURL)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, newOpenAIImageStatusError(resp, "download image bytes failed")
	}
	return io.ReadAll(io.LimitReader(resp.Body, openAIImageMaxDownloadBytes))
}

type openAIImageStatusError struct {
	StatusCode      int
	Message         string
	ResponseBody    []byte
	ResponseHeaders http.Header
	RequestID       string
	URL             string
}

func (e *openAIImageStatusError) Error() string {
	if e == nil {
		return "openai image backend request failed"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("openai image backend request failed: status %d", e.StatusCode)
	}
	return "openai image backend request failed"
}

func newOpenAIImageStatusError(resp *req.Response, fallback string) error {
	if resp == nil {
		if strings.TrimSpace(fallback) == "" {
			fallback = "openai image backend request failed"
		}
		return fmt.Errorf("%s", fallback)
	}

	statusCode := resp.StatusCode
	headers := http.Header(nil)
	requestID := ""
	requestURL := ""
	body := []byte(nil)

	if resp.Response != nil {
		headers = resp.Header.Clone()
		requestID = strings.TrimSpace(resp.Header.Get("x-request-id"))
		if resp.Request != nil && resp.Request.URL != nil {
			requestURL = resp.Request.URL.String()
		}
		if resp.Body != nil {
			body, _ = io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
		}
	}

	message := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(body))
	if message == "" {
		prefix := strings.TrimSpace(fallback)
		if prefix == "" {
			prefix = "openai image backend request failed"
		}
		message = fmt.Sprintf("%s: status %d", prefix, statusCode)
	}

	return &openAIImageStatusError{
		StatusCode:      statusCode,
		Message:         message,
		ResponseBody:    body,
		ResponseHeaders: headers,
		RequestID:       requestID,
		URL:             requestURL,
	}
}

func isOpenAIImageTransientConversationNotFoundError(err error) bool {
	statusErr, ok := err.(*openAIImageStatusError)
	if !ok || statusErr == nil || statusErr.StatusCode != http.StatusNotFound {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(statusErr.Message))
	if strings.Contains(msg, "conversation_not_found") {
		return true
	}
	if strings.Contains(msg, "conversation") && strings.Contains(msg, "not found") {
		return true
	}
	bodyMsg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(statusErr.ResponseBody)))
	if strings.Contains(bodyMsg, "conversation_not_found") {
		return true
	}
	return strings.Contains(bodyMsg, "conversation") && strings.Contains(bodyMsg, "not found")
}

func cloneHTTPHeader(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for key, values := range src {
		copied := make([]string, len(values))
		copy(copied, values)
		dst[key] = copied
	}
	return dst
}

func headerToMap(header http.Header) map[string]string {
	if len(header) == 0 {
		return nil
	}
	result := make(map[string]string, len(header))
	for key, values := range header {
		if len(values) == 0 {
			continue
		}
		result[key] = values[0]
	}
	return result
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
