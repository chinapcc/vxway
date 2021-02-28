package proxy

import "errors"

var(
	ErrUnknownFilter = errors.New("unknown filter")
)

const (
	FilterPrepare = "PREPARE"
	FitterHTTPAccess = "HTTP-ACCESS"
	FilterHeader = "HEADER"
	FilterXForward = "XFORWARD"
	FilterBlackList = "BLACKLIST"
	FilterWhiteList = "WHITELIST"
	FilterAnalysis = "ANALYSIS"
	FilterRateLimiting = "RATE-LIMITING"
	FilterCircuitBreake = "CIRCUIT-BREAKER"

)
