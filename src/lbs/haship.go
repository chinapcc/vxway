package lbs

import (
	"hash/fnv"
	"vxway/src/pb/metapb"
	"vxway/src/utils"

	"github.com/valyala/fasthttp"

)

// HashIPBalance is hash IP loadBalance impl
type HashIPBalance struct {
}

// NewHashIPBalance create a HashIPBalance
func NewHashIPBalance() LoadBalance {
	lb := HashIPBalance{}
	return lb
}

// Select select a server from servers using HashIPBalance
func (haship HashIPBalance) Select(ctx *fasthttp.RequestCtx, servers []metapb.Server) uint64 {
	l := len(servers)
	if 0 >= l {
		return 0
	}
	hash := fnv.New32a()
	// key is client ip
	key := utils.ClientIP(ctx)
	hash.Write([]byte(key))
	serve := servers[hash.Sum32()%uint32(l)]
	return serve.ID
}
