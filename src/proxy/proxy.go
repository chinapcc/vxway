package proxy

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"github.com/valyala/fasthttp"
	"vxway/src/filter"
	"vxway/src/goetty"
	"vxway/src/pb/metapb"
	"vxway/src/plugin"
	"vxway/src/store"
	"vxway/src/utils"
	"vxway/src/utils/expr"
	"vxway/src/utils/hack"
	"vxway/src/utils/task"
	"vxway/src/log"
)

var (
	// MultiResultsContentType 合并设置请求内容数据类型
	MultiResultsContentType = "application/json; charset=utf-8"
	// MultiResultsRemoveHeaders 合并操作需要删除标头
	MultiResultsRemoveHeaders = []string{
		"Content-Length",
		"Content-Type",
		"Date",
	}
)

var (
	globalHTTPOptions *utils.HTTPOption
)

const (
	charLeft  = byte('[')
	charRight = byte(']')
)

// Proxy Proxy
type Proxy struct {
	sync.RWMutex

	dispatchIndex, copyIndex uint64
	dispatches               []chan *dispatchNode
	copies                   []chan *copyReq

	cfg         *Cfg
	filtersMap  map[string]filter.Filter
	filters     []filter.Filter
	client      *utils.FastHTTPClient
	dispatcher  *dispatcher
	rpcListener net.Listener

	jsEngine    *plugin.Engine
	gcJSEngines []*plugin.Engine

	runner   *task.Runner
	stopped  int32
	stopC    chan struct{}
	stopOnce sync.Once
	stopWG   sync.WaitGroup
}

// NewProxy create a new proxy
func NewProxy(cfg *Cfg) *Proxy {
	globalHTTPOptions = &utils.HTTPOption{
		MaxConnDuration:               cfg.Option.LimitDurationConnKeepalive,
		MaxIdleConnDuration:           cfg.Option.LimitDurationConnIdle,
		ReadTimeout:                   cfg.Option.LimitTimeoutRead,
		WriteTimeout:                  cfg.Option.LimitTimeoutWrite,
		MaxResponseBodySize:           cfg.Option.LimitBytesBody,
		WriteBufferSize:               cfg.Option.LimitBufferWrite,
		ReadBufferSize:                cfg.Option.LimitBufferRead,
		MaxConns:                      cfg.Option.LimitCountConn,
		DisableHeaderNamesNormalizing: cfg.Option.DisableHeaderNameNormalizing,
	}

	p := &Proxy{
		client:        utils.NewFastHTTPClientOption(globalHTTPOptions),
		cfg:           cfg,
		filtersMap:    make(map[string]filter.Filter),
		stopC:         make(chan struct{}),
		runner:        task.NewRunner(),
		copies:        make([]chan *copyReq, cfg.Option.LimitCountCopyWorker, cfg.Option.LimitCountCopyWorker),
		dispatches:    make([]chan *dispatchNode, cfg.Option.LimitCountDispatchWorker, cfg.Option.LimitCountDispatchWorker),
		dispatchIndex: 0,
		copyIndex:     0,
		jsEngine:      plugin.NewEngine(cfg.Option.EnableJSPlugin, FilterJSPlugin),
	}

	p.init()

	return p
}

func (p *Proxy) init() {
	err := p.initDispatcher()
	if err != nil {
		log.Fatalf("init route table failed, errors:\n%+v",
			err)
	}

	p.initFilters()

	err = p.dispatcher.store.RegistryProxy(&metapb.Proxy{
		Addr:    p.cfg.Addr,
		AddrRPC: p.cfg.AddrRPC,
	}, p.cfg.TTLProxy)
	if err != nil {
		log.Fatalf("init route table failed, errors:\n%+v",
			err)
	}

	p.dispatcher.load()
}

func (p *Proxy) initDispatcher() error {
	s, err := store.GetStoreFrom(p.cfg.AddrStore, p.cfg.Namespace, p.cfg.AddrStoreUserName, p.cfg.AddrStorePwd)

	if err != nil {
		return err
	}

	p.dispatcher = newDispatcher(p.cfg, s, p.runner, p.updateJSEngine)
	return nil
}

func (p *Proxy) initFilters() {
	for _, filter := range p.cfg.Filers {
		f, err := p.newFilter(filter)
		if nil != err {
			log.Fatalf("create filter failed, filter=<%+v> errors:\n%+v",
				filter,
				err)
		}

		err = f.Init(filter.ExternalCfg)
		if nil != err {
			log.Fatalf("init filter failed, filter=<%+v> errors:\n%+v",
				filter,
				err)
		}

		p.filters = append(p.filters, f)
		p.filtersMap[f.Name()] = f
		log.Infof("filter added, filter=<%s>", f.Name())
	}
}

func (p *Proxy) updateJSEngine(jsEngine *plugin.Engine) {
	var newValues []filter.Filter
	for _, f := range p.filters {
		if f.Name() != FilterJSPlugin {
			newValues = append(newValues, f)
		} else {
			p.addGCJSEngine(f.(*plugin.Engine))
			newValues = append(newValues, jsEngine)
		}
	}

	p.filters = newValues
}

func (p *Proxy) readyToDispatch() {
	for i := uint64(0); i < p.cfg.Option.LimitCountDispatchWorker; i++ {
		c := make(chan *dispatchNode, 1024)
		p.dispatches[i] = c

		_, err := p.runner.RunCancelableTask(func(ctx context.Context) {
			for {
				select {
				case <-ctx.Done():
					return
				case dn := <-c:
					if dn != nil {
						p.doProxy(dn, nil)
					}
				}
			}
		})
		if err != nil {
			log.Fatalf("init dispatch workers failed, errors:\n%+v", err)
		}
	}
}

func (p *Proxy) readyToCopy() {
	for i := uint64(0); i < p.cfg.Option.LimitCountCopyWorker; i++ {
		c := make(chan *copyReq, 1024)
		p.copies[i] = c

		_, err := p.runner.RunCancelableTask(func(ctx context.Context) {
			for {
				select {
				case <-ctx.Done():
					return
				case req := <-c:
					if req != nil {
						p.doCopy(req)
					}
				}
			}
		})
		if err != nil {
			log.Fatalf("init copy workers failed, errors:\n%+v", err)
		}
	}
}

// ServeFastHTTP http reverse handler by fasthttp
func (p *Proxy) ServeFastHTTP(ctx *fasthttp.RequestCtx) {
	var buf bytes.Buffer
	buf.WriteByte(charLeft)
	buf.Write(ctx.Method())
	buf.WriteByte(charRight)
	buf.Write(ctx.RequestURI())
	requestTag := hack.SliceToString(buf.Bytes())

	if p.isStopped() {
		log.Infof("proxy is stopped")
		ctx.SetStatusCode(fasthttp.StatusServiceUnavailable)
		return
	}

	startAt := time.Now()
	api, dispatches, exprCtx := p.dispatcher.dispatch(ctx, requestTag)
	if len(dispatches) == 0 &&
		(nil == api || api.meta.DefaultValue == nil) {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		releaseExprCtx(exprCtx)

		log.Infof("%s: not match, return with 404",
			requestTag)
		return
	}

	// make sure the fasthttp request header has been parsed,
	// avoid concurrent copy header bug
	ctx.Request.Header.Peek("fuck")

	log.Infof("%s: match api %s, has %d dispatches",
		requestTag,
		api.meta.Name,
		len(dispatches))

	rd := acquireRender()
	rd.init(requestTag, api, dispatches)

	var multiCtx *multiContext
	var wg *sync.WaitGroup
	lastBatch := int32(0)
	num := len(dispatches)

	if num > 1 {
		wg = acquireWG()
		multiCtx = acquireMultiContext()
		multiCtx.init()
	}

	for idx, dn := range dispatches {
		// wait last batch complete
		if wg != nil && lastBatch < dn.node.meta.BatchIndex {
			wg.Wait()
			wg = nil
			lastBatch = dn.node.meta.BatchIndex
			if num-idx > 1 {
				wg = &sync.WaitGroup{}
			}
		}

		if wg != nil {
			dn.wg = wg
			wg.Add(1)
		}

		if nil != multiCtx {
			exprCtx.Depend = multiCtx.data
		}
		dn.multiCtx = multiCtx
		dn.requestTag = requestTag
		dn.rd = rd
		dn.ctx = ctx
		if dn.copyTo != nil {
			log.Infof("%s: dispatch node %d copy to %s",
				requestTag,
				idx,
				dn.copyTo.meta.Addr)

			p.copies[getIndex(&p.copyIndex, p.cfg.Option.LimitCountCopyWorker)] <- &copyReq{
				origin:     copyRequest(&ctx.Request),
				to:         dn.copyTo.clone(),
				api:        dn.api.clone(),
				node:       dn.node.clone(),
				idx:        idx,
				params:     exprCtx.CopyParams(),
				requestTag: requestTag,
			}
		}

		if wg != nil {
			p.dispatches[getIndex(&p.dispatchIndex, p.cfg.Option.LimitCountDispatchWorker)] <- dn
		} else {
			p.doProxy(dn, nil)
		}
	}

	// wait last batch complete
	if wg != nil {
		wg.Wait()
		releaseWG(wg)
	}

	rd.render(ctx, multiCtx)
	releaseRender(rd)
	releaseMultiContext(multiCtx)

	incrRequest(api.meta.Name)
	p.postRequest(api, dispatches, startAt)
	releaseExprCtx(exprCtx)

	log.Debugf("%s: dispatch complete",
		requestTag)
}

func (p *Proxy) doCopy(req *copyReq) {
	svr := req.to

	if nil == svr {
		return
	}

	req.prepare()

	log.Infof("%s: dispatch node %d copy to %s",
		req.requestTag,
		req.idx,
		req.to.meta.Addr)

	res, err := p.client.Do(req.origin, svr.meta.Addr, nil)
	if err != nil {
		log.Errorf("%s: dispatch node %d copy to %s with error %s",
			req.requestTag,
			req.idx,
			req.to.meta.Addr,
			err)
		fasthttp.ReleaseRequest(req.origin)
		return
	}

	if res != nil {
		fasthttp.ReleaseResponse(res)
	}

	fasthttp.ReleaseRequest(req.origin)
}

func (p *Proxy) doProxy(dn *dispatchNode, adjustH func(*proxyContext)) {
	if dn.node.meta.UseDefault {
		dn.maybeDone()
		log.Infof("%s: dispatch node %d force using default",
			dn.requestTag,
			dn.idx)
		return
	}

	ctx := dn.ctx
	svr := dn.dest
	if nil == svr {
		dn.err = ErrNoServer
		dn.code = fasthttp.StatusServiceUnavailable
		dn.maybeDone()
		log.Infof("%s: dispatch node %d has no server, return with 503",
			dn.requestTag,
			dn.idx)
		return
	}

	log.Debugf("%s: dispatch node %d to server %d",
		dn.requestTag,
		dn.idx,
		svr.id)

	forwardReq := copyRequest(&ctx.Request)

	// change url
	if dn.needRewrite() {
		// if not use rewrite, it only change uri path and query string
		realPath := expr.Exec(dn.exprCtx, dn.node.parsedExprs...)
		if len(realPath) != 0 {
			log.Infof("%s: dispatch node %d rewrite url to %s",
				dn.requestTag,
				dn.idx,
				hack.SliceToString(realPath))

			forwardReq.SetRequestURIBytes(realPath)
		} else {
			dn.err = ErrRewriteNotMatch
			dn.code = fasthttp.StatusBadRequest
			dn.maybeDone()

			log.Warningf("%s: dispatch node %d rewrite not match, return with 400",
				dn.requestTag,
				dn.idx)
			return
		}
	}

	c := acquireContext()
	c.init(p.dispatcher, ctx, forwardReq, dn)
	if adjustH != nil {
		adjustH(c)
	}

	filters := p.filters

	// pre filters
	filterName, code, err := p.doPreFilters(dn.requestTag, c, filters...)
	if nil != err {
		dn.err = err
		dn.code = code
		dn.maybeDone()
		releaseContext(c)

		log.Errorf("%s: dispatch node %d call filter %s pre failed with error %s",
			dn.requestTag,
			dn.idx,
			filterName,
			err)
		return
	}

	var res *fasthttp.Response

	if value := c.GetAttr(filter.AttrUsingCachingValue); nil != value { // hit cache
		res = fasthttp.AcquireResponse()
		filter.ReadCachedValueTo(value.(*goetty.ByteBuf), res)
		log.Infof("%s: dispatch node %d using cache",
			dn.requestTag,
			dn.idx)
	} else if value := c.GetAttr(filter.AttrUsingResponse); nil != value { // using spec response
		specRes, ok := value.(*fasthttp.Response)
		if !ok {
			dn.err = fmt.Errorf("not support using response attr %T", value)
			dn.code = fasthttp.StatusInternalServerError
			dn.maybeDone()
			releaseContext(c)

			log.Errorf("%s: dispatch node %d using response attr with error %s",
				dn.requestTag,
				dn.idx,
				dn.err)
			return
		}

		log.Infof("%s: dispatch node %d using response attr",
			dn.requestTag,
			dn.idx)
		res = specRes
	} else {
		times := int32(0)
		for {
			log.Infof("%s: dispatch node %d sent for %d times",
				dn.requestTag,
				dn.idx,
				times)

			if !dn.api.isWebSocket() {
				dn.setHost(forwardReq)
				res, err = p.client.Do(forwardReq, svr.meta.Addr, dn.httpOption())
			} else {
				res, err = p.onWebsocket(c, svr.meta.Addr)
			}
			c.setEndAt(time.Now())

			times++

			// skip succeed
			if err == nil && res.StatusCode() < fasthttp.StatusBadRequest {
				break
			}

			// skip no retry strategy
			if !dn.hasRetryStrategy() {
				break
			}

			// skip not match
			if !dn.matchAllRetryStrategy() &&
				!dn.matchRetryStrategy(int32(res.StatusCode())) {
				break
			}

			// retry with strategiess
			retry := dn.retryStrategy()
			if times >= retry.MaxTimes {
				log.Infof("%s: dispatch node %d sent times over the max %d",
					dn.requestTag,
					dn.idx,
					retry.MaxTimes)
				break
			}

			if retry.Interval > 0 {
				time.Sleep(time.Millisecond * time.Duration(retry.Interval))
			}

			fasthttp.ReleaseResponse(res)
			// update selectServer params : change fasthttp.Request to fasthttp.RequestCtx
			p.dispatcher.selectServer(ctx, dn, dn.requestTag)
			svr = dn.dest
			if nil == svr {
				dn.err = ErrNoServer
				dn.code = fasthttp.StatusServiceUnavailable
				dn.maybeDone()

				log.Infof("%s: dispatch node %d has no server, return with 503",
					dn.requestTag,
					dn.idx)
				return
			}
		}
	}

	dn.res = res
	if err != nil || res.StatusCode() >= fasthttp.StatusBadRequest {
		resCode := fasthttp.StatusInternalServerError

		if nil != err {
			log.Errorf("%s: dispatch node %d failed with error %s",
				dn.requestTag,
				dn.idx,
				err)
		} else {
			resCode = res.StatusCode()
			log.Errorf("%s: dispatch node %d failed with error code %d",
				dn.requestTag,
				dn.idx,
				resCode)
		}

		p.doPostErrFilters(c, resCode, err, filters...)

		dn.err = err
		dn.code = resCode
		dn.maybeDone()
		releaseContext(c)
		return
	}

	if log.DebugEnabled() {
		log.Debugf("%s: dispatch node %d return by %s with code %d, body <%s>",
			dn.requestTag,
			dn.idx,
			svr.meta.Addr,
			res.StatusCode(),
			hack.SliceToString(res.Body()))
	}

	// post filters
	filterName, code, err = p.doPostFilters(dn.requestTag, c, filters...)
	if nil != err {
		log.Errorf("%s: dispatch node %d call filter %s post failed with error %s",
			dn.requestTag,
			dn.idx,
			filterName,
			err)

		dn.err = err
		dn.code = code
		dn.maybeDone()
		releaseContext(c)
		return
	}

	dn.maybeDone()
	releaseContext(c)
}

func getIndex(opt *uint64, size uint64) int {
	return int(atomic.AddUint64(opt, 1) % size)
}
