package filter

import (
	"time"

	"github.com/valyala/fasthttp"
	"vxway/src/pb/metapb"
	"vxway/src/utils"
)

// Context filter context
type Context interface {
	StartAt() time.Time
	EndAt() time.Time

	OriginRequest() *fasthttp.RequestCtx
	ForwardRequest() *fasthttp.Request
	Response() *fasthttp.Response

	API() *metapb.API
	DispatchNode() *metapb.DispatchNode
	Server() *metapb.Server
	Analysis() *utils.Analysis

	SetAttr(key string, value interface{})
	GetAttr(key string) interface{}
}

// Filter filter interface
type Filter interface {
	Name() string
	Init(cfg string) error

	Pre(c Context) (statusCode int, err error)
	Post(c Context) (statusCode int, err error)
	PostErr(c Context, code int, err error)
}

// BaseFilter base filter support default implemention
type BaseFilter struct{}

// Init init filter
func (f BaseFilter) Init(cfg string) error {
	return nil
}

// Pre execute before proxy
func (f BaseFilter) Pre(c Context) (statusCode int, err error) {
	return fasthttp.StatusOK, nil
}

// Post execute after proxy
func (f BaseFilter) Post(c Context) (statusCode int, err error) {
	return fasthttp.StatusOK, nil
}

// PostErr execute proxy has errors
func (f BaseFilter) PostErr(c Context, code int, err error) {

}
