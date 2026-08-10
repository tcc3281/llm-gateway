package handler

import (
	"encoding/json"
	"net/http"
)

type OpenAIErrorResponse struct {
	Error OpenAIErrorDetail `json:"error"`
}

type OpenAIErrorDetail struct {
	Message string      `json:"message"`
	Type    string      `json:"type"`
	Param   interface{} `json:"param"`
	Code    string      `json:"code"`
}

type AnthropicErrorResponse struct {
	Type  string               `json:"type"`
	Error AnthropicErrorDetail `json:"error"`
}

type AnthropicErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func WriteErrorResponse(w http.ResponseWriter, inboundFormat string, statusCode int, message, errType, errCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if inboundFormat == "anthropic" {
		antType := "api_error"
		switch statusCode {
		case http.StatusUnauthorized:
			antType = "authentication_error"
		case http.StatusForbidden:
			antType = "permission_error"
		case http.StatusNotFound:
			antType = "not_found_error"
		case http.StatusTooManyRequests:
			antType = "rate_limit_error"
		case http.StatusBadRequest:
			antType = "invalid_request_error"
		}

		json.NewEncoder(w).Encode(AnthropicErrorResponse{
			Type: "error",
			Error: AnthropicErrorDetail{
				Type:    antType,
				Message: message,
			},
		})
		return
	}

	// Default OpenAI Format
	if errType == "" {
		switch statusCode {
		case http.StatusUnauthorized:
			errType = "invalid_request_error"
			if errCode == "" {
				errCode = "invalid_api_key"
			}
		case http.StatusTooManyRequests:
			errType = "requests_error"
			if errCode == "" {
				errCode = "rate_limit_exceeded"
			}
		default:
			errType = "api_error"
		}
	}

	json.NewEncoder(w).Encode(OpenAIErrorResponse{
		Error: OpenAIErrorDetail{
			Message: message,
			Type:    errType,
			Param:   nil,
			Code:    errCode,
		},
	})
}
