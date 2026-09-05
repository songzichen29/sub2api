package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func requestBodyReadErrorMessage(err error) string {
	var readErr *pkghttputil.RequestBodyReadError
	if !errors.As(err, &readErr) {
		return "Failed to read request body"
	}

	switch readErr.Kind {
	case pkghttputil.RequestBodyReadErrorKindUnsupportedEncoding:
		if readErr.Encoding != "" {
			return "Unsupported Content-Encoding: " + readErr.Encoding
		}
		return "Unsupported Content-Encoding"
	case pkghttputil.RequestBodyReadErrorKindDecode:
		if readErr.Encoding != "" {
			return "Failed to decode request body with Content-Encoding: " + readErr.Encoding
		}
		return "Failed to decode request body"
	default:
		return "Failed to read request body"
	}
}

func logRequestBodyReadError(c *gin.Context, component string, err error) {
	fields := []zap.Field{
		zap.Error(err),
		zap.String("client_ip", ip.GetClientIP(c)),
	}
	if c != nil && c.Request != nil {
		req := c.Request
		requestID, _ := req.Context().Value(ctxkey.RequestID).(string)
		clientRequestID, _ := req.Context().Value(ctxkey.ClientRequestID).(string)
		fields = append(fields,
			zap.String("request_id", strings.TrimSpace(requestID)),
			zap.String("client_request_id", strings.TrimSpace(clientRequestID)),
			zap.String("method", req.Method),
			zap.String("path", req.URL.Path),
			zap.Int64("content_length", req.ContentLength),
			zap.String("content_type", req.Header.Get("Content-Type")),
			zap.String("content_encoding", req.Header.Get("Content-Encoding")),
			zap.Strings("transfer_encoding", req.TransferEncoding),
		)
	}

	var readErr *pkghttputil.RequestBodyReadError
	if errors.As(err, &readErr) {
		fields = append(fields,
			zap.String("body_read_error_kind", string(readErr.Kind)),
			zap.String("body_read_content_encoding", readErr.Encoding),
		)
	}

	requestLogger(c, component).Warn("request body read failed", fields...)
}

func writeOpenAIRequestBodyReadError(h *OpenAIGatewayHandler, c *gin.Context, component string, err error) {
	if maxErr, ok := extractMaxBytesError(err); ok {
		h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
		return
	}
	logRequestBodyReadError(c, component, err)
	h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", requestBodyReadErrorMessage(err))
}

func writeOpenAIAnthropicRequestBodyReadError(h *OpenAIGatewayHandler, c *gin.Context, component string, err error) { //nolint:unused // retained for Anthropic-compatible gateway variants
	if maxErr, ok := extractMaxBytesError(err); ok {
		h.anthropicErrorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
		return
	}
	logRequestBodyReadError(c, component, err)
	h.anthropicErrorResponse(c, http.StatusBadRequest, "invalid_request_error", requestBodyReadErrorMessage(err))
}

func writeGatewayRequestBodyReadError(h *GatewayHandler, c *gin.Context, component string, err error) { //nolint:unused // retained for gateway variants
	if maxErr, ok := extractMaxBytesError(err); ok {
		h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
		return
	}
	logRequestBodyReadError(c, component, err)
	h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", requestBodyReadErrorMessage(err))
}

func writeGatewayResponsesRequestBodyReadError(h *GatewayHandler, c *gin.Context, component string, err error) { //nolint:unused // retained for Responses gateway variants
	if maxErr, ok := extractMaxBytesError(err); ok {
		h.responsesErrorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
		return
	}
	logRequestBodyReadError(c, component, err)
	h.responsesErrorResponse(c, http.StatusBadRequest, "invalid_request_error", requestBodyReadErrorMessage(err))
}

func writeGatewayChatCompletionsRequestBodyReadError(h *GatewayHandler, c *gin.Context, component string, err error) { //nolint:unused // retained for chat-completions gateway variants
	if maxErr, ok := extractMaxBytesError(err); ok {
		h.chatCompletionsErrorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
		return
	}
	logRequestBodyReadError(c, component, err)
	h.chatCompletionsErrorResponse(c, http.StatusBadRequest, "invalid_request_error", requestBodyReadErrorMessage(err))
}
