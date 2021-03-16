package proxy

import (
	"errors"

	"github.com/valyala/fasthttp"
	"vxway/src/filter"
)

var (
	// ErrWhitelist target ip not in in white list
	ErrWhitelist = errors.New("Err, target ip not in in white list")
)

// WhiteListFilter whitelist filter
type WhiteListFilter struct {
	filter.BaseFilter
}

func newWhiteListFilter() filter.Filter {
	return &WhiteListFilter{}
}

// Init init filter
func (f *WhiteListFilter) Init(cfg string) error {
	return nil
}

// Name return name of this filter
func (f *WhiteListFilter) Name() string {
	return FilterWhiteList
}

// Pre execute before proxy
func (f *WhiteListFilter) Pre(c filter.Context) (statusCode int, err error) {
	if !c.(*proxyContext).allowWithWhitelist(filter.StringValue(filter.AttrClientRealIP, c)) {
		return fasthttp.StatusForbidden, ErrWhitelist
	}

	return f.BaseFilter.Pre(c)
}
