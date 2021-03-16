package lbs

import (
	"github.com/valyala/fasthttp"
	"vxway/src/pb/metapb"
)

var (
	supportLbs = []metapb.LoadBalance{metapb.RoundRobin}
)

var (
	// LBS map loadBalance name and process function
	LBS = map[metapb.LoadBalance]func() LoadBalance{
		metapb.RoundRobin: NewRoundRobin,
		metapb.WightRobin: NewWeightRobin,
		metapb.IPHash:     NewHashIPBalance,
		metapb.Rand:       NewRandBalance,
	}
)

// LoadBalance loadBalance interface returns selected server's id
type LoadBalance interface {
	Select(ctx *fasthttp.RequestCtx, servers []metapb.Server) uint64
}

// GetSupportLBS return supported loadBalances
func GetSupportLBS() []metapb.LoadBalance {
	return supportLbs
}

// NewLoadBalance create a LoadBalance,if LoadBalance function is not supported
// it will return NewRoundRobin
func NewLoadBalance(name metapb.LoadBalance) LoadBalance {
	if l, ok := LBS[name]; ok {
		return l()
	}
	return NewRoundRobin()
}
