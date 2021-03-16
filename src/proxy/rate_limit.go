package proxy

import (
	"time"
	"vxway/src/pb/metapb"
	"vxway/src/utils/ratelimit"
)

type rateLimiter struct {
	limiter *ratelimit.Bucket
	option  metapb.RateLimitOption
}

func newRateLimiter(max int64, option metapb.RateLimitOption) *rateLimiter {
	return &rateLimiter{
		limiter: ratelimit.NewBucket(time.Second/time.Duration(max), max),
		option:  option,
	}
}

func (l *rateLimiter) do(count int64) bool {
	if l.option == metapb.Wait {
		l.limiter.Wait(count)
		return true
	}

	return l.limiter.TakeAvailable(count) > 0
}
