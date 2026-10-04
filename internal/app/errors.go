package app

import (
	"fmt"
	"time"
)

// Error and warning codes (spec §10). The UI translates them.
const (
	CodeNotAdmin          = "NOT_ADMIN"
	CodePort53Busy        = "PORT53_BUSY"
	CodeNoServers         = "NO_SERVERS"
	CodeEngineSelfTest    = "ENGINE_SELFTEST_FAILED"
	CodeSetDNSFailed      = "SET_DNS_FAILED"
	CodeVerifyLeak        = "VERIFY_LEAK"
	CodeRestoreFailed     = "RESTORE_FAILED"
	CodeDPIStartFailed    = "DPI_START_FAILED"
	CodeDPIBlockedByAV    = "DPI_BLOCKED_BY_AV"
	CodeDPIHashMismatch   = "DPI_HASH_MISMATCH"
	CodeServerListBadSig  = "SERVERLIST_BAD_SIGNATURE"
	CodeUpdateCheckFailed = "UPDATE_CHECK_FAILED"
	CodeAutotuneNoPreset  = "AUTOTUNE_NO_PRESET"
	CodeInternal          = "INTERNAL"
	CodeSettingsReset     = "SETTINGS_RESET"
	CodeNotConnected      = "NOT_CONNECTED"
)

// AppError is a coded error the UI can translate.
type AppError struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
	cause  error
}

// Error is "CODE" or "CODE: cause"; the UI translates the leading code.
func (e *AppError) Error() string {
	if e.cause != nil {
		return e.Code + ": " + e.cause.Error()
	}
	return e.Code
}

func (e *AppError) Unwrap() error { return e.cause }

func appErr(code string, cause error, kv ...any) *AppError {
	e := &AppError{Code: code, cause: cause}
	if len(kv) > 0 {
		e.Params = map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			e.Params[kv[i].(string)] = kv[i+1]
		}
	}
	return e
}

// NoServersError is returned by a Picker that found no working server.
type NoServersError struct {
	Checked int
	Elapsed time.Duration
}

func (e *NoServersError) Error() string {
	return fmt.Sprintf("no working DNS servers (%d checked in %s)", e.Checked, e.Elapsed)
}
