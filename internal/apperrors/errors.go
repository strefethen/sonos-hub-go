package apperrors

// =============================================================================
// Error Codes
// =============================================================================

type ErrorCode string

const (
	ErrorCodeInternalError          ErrorCode = "INTERNAL_ERROR"
	ErrorCodeValidationError        ErrorCode = "VALIDATION_ERROR"
	ErrorCodeNotFound               ErrorCode = "NOT_FOUND"
	ErrorCodeUnauthorized           ErrorCode = "UNAUTHORIZED"
	ErrorCodeForbidden              ErrorCode = "FORBIDDEN"
	ErrorCodeConflict               ErrorCode = "CONFLICT"
	ErrorCodeRateLimited            ErrorCode = "RATE_LIMITED"
	ErrorCodeSonosTimeout           ErrorCode = "SONOS_TIMEOUT"
	ErrorCodeSonosUnreachable       ErrorCode = "SONOS_UNREACHABLE"
	ErrorCodeSonosRejected          ErrorCode = "SONOS_REJECTED"
	ErrorCodeSonosTopology          ErrorCode = "SONOS_TOPOLOGY_CHANGED"
	ErrorCodeSonosVerifyFailed      ErrorCode = "SONOS_VERIFICATION_FAILED"
	ErrorCodeDeviceNotFound         ErrorCode = "DEVICE_NOT_FOUND"
	ErrorCodeDeviceOffline          ErrorCode = "DEVICE_OFFLINE"
	ErrorCodeDeviceNotTarget        ErrorCode = "DEVICE_NOT_TARGETABLE"
	ErrorCodeSceneNotFound          ErrorCode = "SCENE_NOT_FOUND"
	ErrorCodeSceneLockHeld          ErrorCode = "SCENE_LOCK_HELD"
	ErrorCodeSceneExecFailed        ErrorCode = "SCENE_EXECUTION_FAILED"
	ErrorCodeSceneCoordMissing      ErrorCode = "SCENE_COORDINATOR_UNAVAILABLE"
	ErrorCodeRoutineNotFound        ErrorCode = "ROUTINE_NOT_FOUND"
	ErrorCodeRoutineSkipped         ErrorCode = "ROUTINE_SKIPPED"
	ErrorCodeJobNotFound            ErrorCode = "JOB_NOT_FOUND"
	ErrorCodeHolidayNotFound        ErrorCode = "HOLIDAY_NOT_FOUND"
	ErrorCodeEventNotFound          ErrorCode = "EVENT_NOT_FOUND"
	ErrorCodeInvalidEventType       ErrorCode = "INVALID_EVENT_TYPE"
	ErrorCodeInvalidSchedule        ErrorCode = "INVALID_SCHEDULE"
	ErrorCodeMusicHandleMissing     ErrorCode = "MUSIC_HANDLE_NOT_FOUND"
	ErrorCodeCuratedSetNotFound     ErrorCode = "CURATED_SET_NOT_FOUND"
	ErrorCodeCuratedSetEmpty        ErrorCode = "CURATED_SET_EMPTY"
	ErrorCodeSetNotFound            ErrorCode = "SET_NOT_FOUND"
	ErrorCodeItemNotFound           ErrorCode = "ITEM_NOT_FOUND"
	ErrorCodeEmptySet               ErrorCode = "EMPTY_SET"
	ErrorCodeAppleTokenExpired      ErrorCode = "APPLE_TOKEN_EXPIRED"
	ErrorCodeAppleTokenInvalid      ErrorCode = "APPLE_TOKEN_INVALID"
	ErrorCodeAppleAPIError          ErrorCode = "APPLE_API_ERROR"
	ErrorCodeAuthPairingExpired     ErrorCode = "AUTH_PAIRING_EXPIRED"
	ErrorCodeAuthPairingInvalid     ErrorCode = "AUTH_PAIRING_INVALID"
	ErrorCodeAuthTokenExpired       ErrorCode = "AUTH_TOKEN_EXPIRED"
	ErrorCodeAuthTokenInvalid       ErrorCode = "AUTH_TOKEN_INVALID"
	ErrorCodeServiceNotBootstrapped ErrorCode = "SERVICE_NOT_BOOTSTRAPPED"
	ErrorCodeServiceAuthFailed      ErrorCode = "SERVICE_AUTH_FAILED"
	ErrorCodeContentTypeUnsupported ErrorCode = "CONTENT_TYPE_UNSUPPORTED"
	ErrorCodeContentUnavailable     ErrorCode = "CONTENT_UNAVAILABLE"
)

// Remediation provides guidance on how to fix an error.
type Remediation struct {
	Action     string `json:"action"`
	Endpoint   string `json:"endpoint,omitempty"`
	UserAction string `json:"user_action,omitempty"`
}

// ErrorBody is the serialized error payload.
// Deprecated: Use StripeErrorBody for Stripe API-style errors.
type ErrorBody struct {
	Code        ErrorCode      `json:"code"`
	Message     string         `json:"message"`
	Details     map[string]any `json:"details,omitempty"`
	Remediation *Remediation   `json:"remediation,omitempty"`
}

// =============================================================================
// Stripe API Error Types
// =============================================================================

// ErrorType categorizes errors following Stripe API conventions.
type ErrorType string

const (
	// ErrorTypeInvalidRequest indicates invalid parameters, missing required fields, etc.
	ErrorTypeInvalidRequest ErrorType = "invalid_request_error"
	// ErrorTypeAPIError indicates an internal API error.
	ErrorTypeAPIError ErrorType = "api_error"
	// ErrorTypeAuthError indicates authentication or authorization failure.
	ErrorTypeAuthError ErrorType = "authentication_error"
)

// StripeErrorBody is the Stripe-style error payload.
// Format: {"type": "invalid_request_error", "code": "NOT_FOUND", "message": "..."}
type StripeErrorBody struct {
	Type    ErrorType `json:"type"`
	Code    string    `json:"code"`
	Message string    `json:"message"`
}

// AppError is the base error type for HTTP responses.
type AppError struct {
	Code        ErrorCode
	Message     string
	StatusCode  int
	Details     map[string]any
	Remediation *Remediation
}

func (err *AppError) Error() string {
	return err.Message
}

func (err *AppError) ErrorBody() ErrorBody {
	body := ErrorBody{
		Code:    err.Code,
		Message: err.Message,
	}
	if err.Details != nil {
		body.Details = err.Details
	}
	if err.Remediation != nil {
		body.Remediation = err.Remediation
	}
	return body
}

// StripeErrorBody returns the error in Stripe API format.
func (err *AppError) StripeErrorBody() StripeErrorBody {
	// Map status code to error type
	errType := ErrorTypeAPIError
	switch {
	case err.StatusCode >= 400 && err.StatusCode < 500:
		errType = ErrorTypeInvalidRequest
	case err.StatusCode == 401 || err.StatusCode == 403:
		errType = ErrorTypeAuthError
	}

	return StripeErrorBody{
		Type:    errType,
		Code:    string(err.Code),
		Message: err.Message,
	}
}

func NewAppError(code ErrorCode, message string, statusCode int, details map[string]any, remediation *Remediation) *AppError {
	return &AppError{
		Code:        code,
		Message:     message,
		StatusCode:  statusCode,
		Details:     details,
		Remediation: remediation,
	}
}

func NewValidationError(message string, details map[string]any) *AppError {
	return NewAppError(ErrorCodeValidationError, message, 400, details, nil)
}

func NewUnauthorizedError(message string, code ...ErrorCode) *AppError {
	errCode := ErrorCodeUnauthorized
	if len(code) > 0 {
		errCode = code[0]
	}
	return NewAppError(errCode, message, 401, nil, nil)
}

func NewForbiddenError(message string) *AppError {
	return NewAppError(ErrorCodeForbidden, message, 403, nil, nil)
}

func NewNotFoundError(message string, details map[string]any) *AppError {
	return NewAppError(ErrorCodeNotFound, message, 404, details, nil)
}

func NewNotFoundResource(resource, id string) *AppError {
	message := resource + " not found"
	details := map[string]any{
		"resource": resource,
	}
	if id != "" {
		message = resource + " not found: " + id
		details["id"] = id
	}
	return NewAppError(ErrorCodeNotFound, message, 404, details, nil)
}

func NewConflictError(message string, details map[string]any) *AppError {
	return NewAppError(ErrorCodeConflict, message, 409, details, nil)
}

func NewRateLimitError(message string) *AppError {
	return NewAppError(ErrorCodeRateLimited, message, 429, nil, nil)
}

func NewInternalError(message string) *AppError {
	return NewAppError(ErrorCodeInternalError, message, 500, nil, nil)
}

// EnsureAppError converts an arbitrary error into an AppError.
func EnsureAppError(err error) *AppError {
	if err == nil {
		return NewInternalError("Unknown error")
	}
	if appErr, ok := err.(*AppError); ok {
		return appErr
	}
	return NewInternalError("Internal server error")
}
