package synthetic

import "air-traffic/internal/model"

// vendorError returns the HTTP status and the byte-identical error envelope for a
// given vendor. Each vendor's real API uses a distinct error shape; we reproduce it.
func vendorError(vendorID string, status int, code, message string) (int, any) {
	switch vendorID {
	case "openai", "mistral", "groq", "together", "cohere", "perplexity", "xai":
		// OpenAI-style error envelope (the de-facto standard these vendors follow).
		return status, map[string]any{
			"error": map[string]any{
				"message": message,
				"type":    code,
				"param":   nil,
				"code":    code,
			},
		}
	case "anthropic":
		return status, map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    code,
				"message": message,
			},
		}
	case "bedrock", "amazon_q":
		// AWS JSON protocol error shape.
		return status, map[string]any{
			"__type":  awsExceptionName(code),
			"message": message,
		}
	case "azure_openai":
		return status, map[string]any{
			"error": map[string]any{
				"code":    azureCode(code),
				"message": message,
			},
		}
	case "vertex":
		// Google API error envelope.
		return status, map[string]any{
			"error": map[string]any{
				"code":    status,
				"message": message,
				"status":  googleStatus(code),
			},
		}
	case "github_copilot":
		return status, map[string]any{
			"message":           message,
			"documentation_url": "https://docs.github.com/rest/copilot",
			"status":            itoa(status),
		}
	case "databricks":
		return status, map[string]any{
			"error_code": dbxCode(code),
			"message":    message,
		}
	case "m365_copilot":
		return status, map[string]any{
			"error": map[string]any{
				"code":    msGraphCode(code),
				"message": message,
			},
		}
	case "watsonx":
		return status, map[string]any{
			"errors": []any{map[string]any{"code": code, "message": message}},
			"trace":  model.NewUUID(),
		}
	default:
		return status, map[string]any{"error": map[string]any{"message": message, "type": code, "code": code}}
	}
}

// emptyEnvelope reproduces an empty-but-valid list response per vendor shape.
func emptyEnvelope(vendorID string) any {
	switch vendorID {
	case "openai", "mistral":
		return map[string]any{"object": "list", "data": []any{}, "has_more": false, "first_id": nil, "last_id": nil}
	case "anthropic":
		return map[string]any{"data": []any{}, "has_more": false, "first_id": nil, "last_id": nil}
	case "github_copilot":
		return []any{}
	case "bedrock":
		return map[string]any{"results": []any{}}
	case "vertex":
		return map[string]any{}
	default:
		return map[string]any{"data": []any{}}
	}
}

// genericFixture is the fallback for vendors without a deep fixture (Tier 2/3):
// a plausible, vendor-shaped success envelope echoing the request path.
func genericFixture(a model.Adapter, method, path string, query map[string][]string) (int, any, map[string]string) {
	body := map[string]any{
		"object":      "list",
		"data":        []any{},
		"has_more":    false,
		"vendor":      a.Vendor,
		"api_version": a.APIVersion,
		"path":        path,
		"note":        "synthetic surface (Tier " + itoa(a.Tier) + "); manifest-level fidelity",
	}
	return 200, body, nil
}

func awsExceptionName(code string) string {
	switch code {
	case "unauthorized", "forbidden":
		return "AccessDeniedException"
	case "rate_limit_exceeded":
		return "ThrottlingException"
	case "service_unavailable":
		return "ServiceUnavailableException"
	default:
		return "InternalServerException"
	}
}

func azureCode(code string) string {
	switch code {
	case "unauthorized":
		return "401"
	case "forbidden":
		return "AuthorizationFailed"
	case "rate_limit_exceeded":
		return "429"
	default:
		return "InternalServerError"
	}
}

func googleStatus(code string) string {
	switch code {
	case "unauthorized":
		return "UNAUTHENTICATED"
	case "forbidden":
		return "PERMISSION_DENIED"
	case "rate_limit_exceeded":
		return "RESOURCE_EXHAUSTED"
	case "timeout":
		return "DEADLINE_EXCEEDED"
	default:
		return "INTERNAL"
	}
}

func dbxCode(code string) string {
	switch code {
	case "unauthorized":
		return "PERMISSION_DENIED"
	case "rate_limit_exceeded":
		return "REQUEST_LIMIT_EXCEEDED"
	default:
		return "INTERNAL_ERROR"
	}
}

func msGraphCode(code string) string {
	switch code {
	case "unauthorized":
		return "InvalidAuthenticationToken"
	case "forbidden":
		return "accessDenied"
	case "rate_limit_exceeded":
		return "activityLimitReached"
	default:
		return "generalException"
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
