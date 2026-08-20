package errs

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cloud-print/server/internal/domain"
)

type ErrorCode string

const (
	ErrAuthInvalid         ErrorCode = "AUTH_INVALID"
	ErrJWTExpired          ErrorCode = "JWT_EXPIRED"
	ErrNoPermission        ErrorCode = "NO_PERMISSION"
	ErrUsernameExists      ErrorCode = "USERNAME_EXISTS"
	ErrCredentialInvalid   ErrorCode = "CREDENTIAL_INVALID"
	ErrAdminRequired       ErrorCode = "ADMIN_REQUIRED"
	ErrAuthMissing         ErrorCode = "AUTH_MISSING"

	ErrDeviceNotFound      ErrorCode = "DEVICE_NOT_FOUND"
	ErrAgentOffline        ErrorCode = "AGENT_OFFLINE"
	ErrDeviceProbeFailed   ErrorCode = "DEVICE_PROBE_FAILED"
	ErrTaskNotCancellable  ErrorCode = "TASK_NOT_CANCELLABLE"
	ErrDeviceOffline       ErrorCode = "DEVICE_OFFLINE"
	ErrDeviceIDConflict    ErrorCode = "DEVICE_ID_CONFLICT"
	ErrDeviceFieldInvalid  ErrorCode = "DEVICE_FIELD_INVALID"

	ErrDocTooLarge         ErrorCode = "DOC_TOO_LARGE"
	ErrFormatUnsupported   ErrorCode = "FORMAT_UNSUPPORTED"

	ErrParamInvalid        ErrorCode = "PARAM_INVALID"
	ErrFactoryHasDependents ErrorCode = "FACTORY_HAS_DEPENDENTS"
	ErrAgentIDExists       ErrorCode = "AGENT_ID_EXISTS"
	ErrConfigInvalid       ErrorCode = "CONFIG_INVALID"
	ErrConfigMissing       ErrorCode = "CONFIG_MISSING"
	ErrConfigVersion       ErrorCode = "CONFIG_VERSION"
	ErrAgentOfflineUpgrade ErrorCode = "AGENT_OFFLINE_UPGRADE"

	ErrInternalError       ErrorCode = "INTERNAL_ERROR"

	ErrDNSResolveFail          ErrorCode = "DNS_RESOLVE_FAIL"
	ErrCloudGatewayUnreachable ErrorCode = "CLOUD_GATEWAY_UNREACHABLE"
	ErrLocalNetFail            ErrorCode = "LOCAL_NET_FAIL"
	ErrTLSVerifyFail           ErrorCode = "TLS_VERIFY_FAIL"
	ErrDomainUpdateFail        ErrorCode = "DOMAIN_UPDATE_FAIL"
	ErrWSSHandshakeFail        ErrorCode = "WSS_HANDSHAKE_FAIL"
	ErrCloudDisconnected       ErrorCode = "CLOUD_DISCONNECTED"

	ErrTaskDataInvalid ErrorCode = "TASK_DATA_INVALID"
	ErrTaskNotFound    ErrorCode = "TASK_NOT_FOUND"
	ErrTaskCancelFail  ErrorCode = "TASK_CANCEL_FAIL"
	ErrTaskTimeout     ErrorCode = "TASK_TIMEOUT"

	ErrQueueFull  ErrorCode = "QUEUE_FULL"
	ErrQueueEmpty ErrorCode = "QUEUE_EMPTY"

	ErrCredentialMissing ErrorCode = "CREDENTIAL_MISSING"

	ErrStorageIO      ErrorCode = "STORAGE_IO"
	ErrStorageCorrupt ErrorCode = "STORAGE_CORRUPT"

	ErrProtocolProbeFail ErrorCode = "PROTOCOL_PROBE_FAIL"
	ErrProtocolSendFail  ErrorCode = "PROTOCOL_SEND_FAIL"

	ErrOpsAccessDenied ErrorCode = "OPS_ACCESS_DENIED"

	ErrUpgradeDownload ErrorCode = "UPGRADE_DOWNLOAD"
	ErrUpgradeVerify   ErrorCode = "UPGRADE_VERIFY"
	ErrUpgradeRollback ErrorCode = "UPGRADE_ROLLBACK"
)

type APIError struct {
	Code       ErrorCode
	Message    string
	Cause      error
	HTTPStatus int
	NetClass   domain.NetClass
}

func (e *APIError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *APIError) Unwrap() error { return e.Cause }

func (e *APIError) Is(target error) bool {
	var t *APIError
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
}

func New(code ErrorCode, message string) *APIError {
	return &APIError{Code: code, Message: message, HTTPStatus: HTTPStatusFor(code)}
}

func Newf(code ErrorCode, format string, args ...interface{}) *APIError {
	return &APIError{Code: code, Message: fmt.Sprintf(format, args...), HTTPStatus: HTTPStatusFor(code)}
}

func Wrap(code ErrorCode, message string, cause error) *APIError {
	return &APIError{Code: code, Message: message, Cause: cause, HTTPStatus: HTTPStatusFor(code)}
}

func Wrapf(code ErrorCode, cause error, format string, args ...interface{}) *APIError {
	return &APIError{Code: code, Message: fmt.Sprintf(format, args...), Cause: cause, HTTPStatus: HTTPStatusFor(code)}
}

func (e *APIError) WithNetClass(class domain.NetClass) *APIError {
	e.NetClass = class
	return e
}

func NewNetError(code ErrorCode, message string, class domain.NetClass) *APIError {
	return &APIError{Code: code, Message: message, NetClass: class, HTTPStatus: HTTPStatusFor(code)}
}

func WrapNetError(code ErrorCode, message string, cause error, class domain.NetClass) *APIError {
	return &APIError{Code: code, Message: message, Cause: cause, NetClass: class, HTTPStatus: HTTPStatusFor(code)}
}

func HTTPStatusFor(code ErrorCode) int {
	switch code {
	case ErrAuthInvalid, ErrJWTExpired, ErrAuthMissing, ErrCredentialInvalid:
		return http.StatusUnauthorized
	case ErrNoPermission, ErrAdminRequired, ErrOpsAccessDenied:
		return http.StatusForbidden
	case ErrDeviceNotFound, ErrTaskNotFound, ErrCredentialMissing:
		return http.StatusNotFound
	case ErrUsernameExists, ErrAgentIDExists, ErrDeviceIDConflict, ErrConfigInvalid, ErrParamInvalid:
		return http.StatusBadRequest
	case ErrDocTooLarge:
		return http.StatusRequestEntityTooLarge
	case ErrFormatUnsupported, ErrTaskDataInvalid, ErrDeviceFieldInvalid:
		return http.StatusUnprocessableEntity
	case ErrTaskNotCancellable, ErrTaskCancelFail, ErrFactoryHasDependents, ErrAgentOfflineUpgrade, ErrDeviceProbeFailed:
		return http.StatusConflict
	case ErrAgentOffline, ErrDeviceOffline, ErrCloudDisconnected:
		return http.StatusServiceUnavailable
	case ErrInternalError, ErrStorageIO, ErrStorageCorrupt:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

func IsRetryable(err error) bool {
	var ae *APIError
	if !errors.As(err, &ae) {
		return false
	}
	switch ae.Code {
	case ErrCloudGatewayUnreachable,
		ErrDNSResolveFail,
		ErrLocalNetFail,
		ErrWSSHandshakeFail,
		ErrCloudDisconnected,
		ErrDeviceOffline,
		ErrAgentOffline,
		ErrTaskTimeout,
		ErrProtocolSendFail,
		ErrStorageIO:
		return true
	default:
		return false
	}
}

func IsTerminal(err error) bool {
	var ae *APIError
	if !errors.As(err, &ae) {
		return false
	}
	switch ae.Code {
	case ErrAuthInvalid,
		ErrAuthMissing,
		ErrTaskDataInvalid,
		ErrTaskNotFound,
		ErrTaskCancelFail,
		ErrDeviceNotFound,
		ErrDeviceIDConflict,
		ErrDeviceFieldInvalid,
		ErrQueueFull,
		ErrConfigInvalid,
		ErrConfigMissing,
		ErrCredentialInvalid,
		ErrCredentialMissing,
		ErrStorageCorrupt,
		ErrProtocolProbeFail,
		ErrOpsAccessDenied,
		ErrTLSVerifyFail:
		return true
	default:
		return false
	}
}

func IsNetError(err error) bool {
	var ae *APIError
	if !errors.As(err, &ae) {
		return false
	}
	switch ae.Code {
	case ErrDNSResolveFail,
		ErrCloudGatewayUnreachable,
		ErrLocalNetFail,
		ErrCloudDisconnected,
		ErrWSSHandshakeFail:
		return true
	default:
		return false
	}
}

func NetClassFromErr(err error) domain.NetClass {
	var ae *APIError
	if !errors.As(err, &ae) {
		return domain.NetClassOK
	}
	if ae.NetClass != "" {
		return ae.NetClass
	}
	switch ae.Code {
	case ErrDNSResolveFail:
		return domain.NetClassDNSResolveFail
	case ErrCloudGatewayUnreachable:
		return domain.NetClassCloudGatewayUnreachable
	case ErrLocalNetFail:
		return domain.NetClassLocalNetFail
	default:
		return domain.NetClassOK
	}
}

func CodeFromErr(err error) ErrorCode {
	var ae *APIError
	if !errors.As(err, &ae) {
		return ""
	}
	return ae.Code
}