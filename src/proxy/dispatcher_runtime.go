package proxy

import (
	"github.com/valyala/fasthttp"

	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"vxway/src/goetty"
	"vxway/src/lbs"
	"vxway/src/log"
	"vxway/src/pb/metapb"
	"vxway/src/utils"
	"vxway/src/utils/hack"
	"vxway/src/utils/expr"

	pbutil "vxway/src/utils/protoc"
	jsonparser "vxway/src/json"
)

var (
	dependP = regexp.MustCompile(`\$\w+\.\w+`)
)

type binds struct {
	servers []*bindInfo
	actives []metapb.Server
}

type bindInfo struct {
	svrID  uint64
	status metapb.Status
}

type clusterRuntime struct {
	meta *metapb.Cluster
	lb   lbs.LoadBalance
}

func newClusterRuntime(meta *metapb.Cluster) *clusterRuntime {
	return &clusterRuntime{
		meta: meta,
		lb:   lbs.NewLoadBalance(meta.LoadBalance),
	}
}

func (c *clusterRuntime) clone() *clusterRuntime {
	meta := &metapb.Cluster{}
	pbutil.MustUnmarshal(meta, pbutil.MustMarshal(c.meta))
	return newClusterRuntime(meta)
}

func (c *clusterRuntime) updateMeta(meta *metapb.Cluster) {
	c.meta = meta
	c.lb = lbs.NewLoadBalance(meta.LoadBalance)
}

func (c *clusterRuntime) selectServer(req *fasthttp.RequestCtx, svrs []metapb.Server) uint64 {
	return c.lb.Select(req, svrs)
}

type abstractSupportProtectedRuntime struct {
	sync.RWMutex

	id        uint64
	tw        *goetty.TimeoutWheel
	activeQPS int64
	limiter   *rateLimiter
	circuit   metapb.CircuitStatus
	cb        *metapb.CircuitBreaker
	barrier   *utils.RateBarrier
}

func (s *abstractSupportProtectedRuntime) getCircuitStatus() metapb.CircuitStatus {
	s.RLock()
	value := s.circuit
	s.RUnlock()
	return value
}

func (s *abstractSupportProtectedRuntime) circuitToClose() {
	s.Lock()
	if s.cb == nil ||
		s.circuit == metapb.Close {
		s.Unlock()
		return
	}

	s.circuit = metapb.Close

	log.Warningf("protected resource <%d> change to close", s.id)
	s.tw.Schedule(time.Duration(s.cb.CloseTimeout), s.circuitToHalf, nil)
	s.Unlock()
}

func (s *abstractSupportProtectedRuntime) circuitToOpen() {
	s.Lock()
	if s.cb == nil ||
		s.circuit == metapb.Open ||
		s.circuit != metapb.Half {
		s.Unlock()
		return
	}

	s.circuit = metapb.Open
	log.Infof("protected resource <%d> change to open", s.id)
	s.Unlock()
}

func (s *abstractSupportProtectedRuntime) circuitToHalf(arg interface{}) {
	s.Lock()
	if s.cb != nil {
		s.circuit = metapb.Half
		log.Warningf("protected resource <%d> change to half", s.id)
	}
	s.Unlock()
}

type serverRuntime struct {
	abstractSupportProtectedRuntime

	meta             *metapb.Server
	heathTimeout     goetty.Timeout
	checkFailCount   int
	useCheckDuration time.Duration
}

func newServerRuntime(meta *metapb.Server, tw *goetty.TimeoutWheel, activeQPS int64) *serverRuntime {
	rt := &serverRuntime{}
	rt.tw = tw
	rt.activeQPS = activeQPS
	rt.updateMeta(meta)
	return rt
}

func (s *serverRuntime) clone() *serverRuntime {
	meta := &metapb.Server{}
	pbutil.MustUnmarshal(meta, pbutil.MustMarshal(s.meta))
	return newServerRuntime(meta, s.tw, s.activeQPS)
}

func (s *serverRuntime) updateMeta(meta *metapb.Server) {
	s.heathTimeout.Stop()
	activeQPS := s.activeQPS
	tw := s.tw

	*s = serverRuntime{}
	s.tw = tw
	s.activeQPS = activeQPS
	s.meta = meta
	s.id = meta.ID
	s.cb = meta.CircuitBreaker
	s.limiter = newRateLimiter(s.activeQPS, s.meta.RateLimitOption)
	s.circuit = metapb.Open
	if s.cb != nil {
		s.barrier = utils.NewRateBarrier(int(s.cb.HalfTrafficRate))
	}
}

func (s *serverRuntime) getCheckURL() string {
	return fmt.Sprintf("%s://%s%s", strings.ToLower(s.meta.Protocol.String()), s.meta.Addr, s.meta.HeathCheck.Path)
}

func (s *serverRuntime) fail() {
	s.checkFailCount++
	s.useCheckDuration += s.useCheckDuration / 2
}

func (s *serverRuntime) reset() {
	s.checkFailCount = 0
	s.useCheckDuration = time.Duration(s.meta.HeathCheck.CheckInterval)
}

type ipSegment struct {
	value []string
}

func parseFrom(value string) *ipSegment {
	ip := &ipSegment{}
	ip.value = strings.Split(value, ".")
	return ip
}

func (ip *ipSegment) matches(value string) bool {
	tmp := strings.Split(value, ".")

	for index, v := range ip.value {
		if v != "*" && v != tmp[index] {
			return false
		}
	}

	return true
}

type apiValidation struct {
	meta  *metapb.Validation
	rules []*apiRule
}

type apiRule struct {
	pattern *regexp.Regexp
}

type apiNode struct {
	httpOption     utils.HTTPOption
	meta           *metapb.DispatchNode
	validations    []*apiValidation
	defaultCookies []*fasthttp.Cookie
	parsedExprs    []expr.Expr
}

func newAPINode(meta *metapb.DispatchNode) *apiNode {
	rn := &apiNode{
		meta: meta,
	}

	if meta.URLRewrite != "" {
		exprs, err := expr.Parse([]byte(strings.TrimSpace(meta.URLRewrite)))
		if err != nil {
			log.Fatalf("bug: parse url rewrite expr failed with error %+v", err)
		}
		rn.parsedExprs = exprs
	}

	if nil != meta.DefaultValue {
		for _, c := range meta.DefaultValue.Cookies {
			ck := &fasthttp.Cookie{}
			ck.SetKey(c.Name)
			ck.SetValue(c.Value)
			rn.defaultCookies = append(rn.defaultCookies, ck)
		}
	}

	for _, v := range meta.Validations {
		rv := &apiValidation{
			meta: v,
		}

		for _, r := range v.Rules {
			rv.rules = append(rv.rules, &apiRule{
				pattern: regexp.MustCompile(r.Expression),
			})
		}

		rn.validations = append(rn.validations, rv)
	}

	rn.httpOption = *globalHTTPOptions
	if meta.ReadTimeout > 0 {
		rn.httpOption.ReadTimeout = time.Duration(meta.ReadTimeout)
	}
	if meta.WriteTimeout > 0 {
		rn.httpOption.WriteTimeout = time.Duration(meta.WriteTimeout)
	}
	return rn
}

func (n *apiNode) clone() *apiNode {
	meta := &metapb.DispatchNode{}
	pbutil.MustUnmarshal(meta, pbutil.MustMarshal(n.meta))
	return newAPINode(meta)
}

func (n *apiNode) validate(req *fasthttp.Request) bool {
	if len(n.validations) == 0 {
		return true
	}

	for _, v := range n.validations {
		if !v.validate(req) {
			return false
		}
	}

	return true
}

type renderAttr struct {
	meta     *metapb.RenderAttr
	extracts [][]string
}

type renderObject struct {
	meta  *metapb.RenderObject
	attrs []*renderAttr
}

type apiRuntime struct {
	abstractSupportProtectedRuntime

	meta                *metapb.API
	nodes               []*apiNode
	defaultCookies      []*fasthttp.Cookie
	parsedWhitelist     []*ipSegment
	parsedBlacklist     []*ipSegment
	parsedRenderObjects []*renderObject
}

func newAPIRuntime(meta *metapb.API, tw *goetty.TimeoutWheel, activeQPS int64) *apiRuntime {
	ar := &apiRuntime{
		meta: meta,
	}
	ar.activeQPS = activeQPS
	ar.tw = tw
	ar.init()

	return ar
}

func (a *apiRuntime) clone() *apiRuntime {
	meta := &metapb.API{}
	pbutil.MustUnmarshal(meta, pbutil.MustMarshal(a.meta))
	return newAPIRuntime(meta, a.tw, a.activeQPS)
}

func (a *apiRuntime) updateMeta(meta *metapb.API) {
	tw := a.tw
	activeQPS := a.activeQPS

	*a = apiRuntime{}
	a.meta = meta
	a.tw = tw
	a.activeQPS = activeQPS
	a.init()
}

func (a *apiRuntime) compare(i, j int) bool {
	return a.nodes[i].meta.BatchIndex-a.nodes[j].meta.BatchIndex < 0
}

func (a *apiRuntime) init() {
	for _, n := range a.meta.Nodes {
		a.nodes = append(a.nodes, newAPINode(n))
	}

	sort.Slice(a.nodes, a.compare)

	if nil != a.meta.DefaultValue {
		for _, c := range a.meta.DefaultValue.Cookies {
			ck := &fasthttp.Cookie{}
			ck.SetKey(c.Name)
			ck.SetValue(c.Value)
			a.defaultCookies = append(a.defaultCookies, ck)
		}
	}

	a.parsedWhitelist = make([]*ipSegment, 0)
	a.parsedBlacklist = make([]*ipSegment, 0)
	if nil != a.meta.IPAccessControl {
		if a.meta.IPAccessControl.Whitelist != nil {
			for _, ip := range a.meta.IPAccessControl.Whitelist {
				a.parsedWhitelist = append(a.parsedWhitelist, parseFrom(ip))
			}
		}

		if a.meta.IPAccessControl.Blacklist != nil {
			for _, ip := range a.meta.IPAccessControl.Blacklist {
				a.parsedBlacklist = append(a.parsedBlacklist, parseFrom(ip))
			}
		}
	}

	if nil != a.meta.RenderTemplate {
		for _, obj := range a.meta.RenderTemplate.Objects {
			rob := &renderObject{
				meta: obj,
			}

			for _, attr := range obj.Attrs {
				rattr := &renderAttr{
					meta: attr,
				}
				rob.attrs = append(rob.attrs, rattr)

				extracts := strings.Split(attr.ExtractExp, ",")
				for _, extract := range extracts {
					rattr.extracts = append(rattr.extracts, strings.Split(extract, "."))
				}
			}

			a.parsedRenderObjects = append(a.parsedRenderObjects, rob)
		}
	}

	a.id = a.meta.ID
	a.cb = a.meta.CircuitBreaker
	a.circuit = metapb.Open
	if a.cb != nil {
		a.barrier = utils.NewRateBarrier(int(a.cb.HalfTrafficRate))
	}
	if a.meta.MaxQPS > 0 {
		a.limiter = newRateLimiter(a.activeQPS, a.meta.RateLimitOption)
	}

	return
}

func (a *apiRuntime) isWebSocket() bool {
	return a.meta.WebSocketOptions != nil
}

func (a *apiRuntime) webSocketOptions() *metapb.WebSocketOptions {
	return a.meta.WebSocketOptions
}

func (a *apiRuntime) hasRenderTemplate() bool {
	return a.meta.RenderTemplate != nil
}

func (a *apiRuntime) hasDefaultValue() bool {
	return a.meta.DefaultValue != nil
}

func (a *apiRuntime) allowWithBlacklist(ip string) bool {
	if a.meta.IPAccessControl == nil {
		return true
	}

	for _, i := range a.parsedBlacklist {
		if i.matches(ip) {
			return false
		}
	}

	return true
}

func (a *apiRuntime) allowWithWhitelist(ip string) bool {
	if a.meta.IPAccessControl == nil || len(a.meta.IPAccessControl.Whitelist) == 0 {
		return true
	}

	for _, i := range a.parsedWhitelist {
		if i.matches(ip) {
			return true
		}
	}

	return false
}

func (a *apiRuntime) isUp() bool {
	return a.meta.Status == metapb.Up
}

func (a *apiRuntime) matches(req *fasthttp.Request) bool {
	if !a.isUp() {
		return false
	}

	switch a.matchRule() {
	case metapb.MatchAll:
		return a.isDomainMatches(req) && a.isMethodMatches(req)
	case metapb.MatchAny:
		return a.isDomainMatches(req) || a.isMethodMatches(req)
	default:
		return a.isDomainMatches(req) || a.isMethodMatches(req)
	}
}

func (a *apiRuntime) isMethodMatches(req *fasthttp.Request) bool {
	return a.meta.Method == "*" || strings.ToUpper(hack.SliceToString(req.Header.Method())) == a.meta.Method
}

func (a *apiRuntime) isDomainMatches(req *fasthttp.Request) bool {
	return a.meta.Domain != "" && hack.SliceToString(req.Header.Host()) == a.meta.Domain
}

func (a *apiRuntime) position() uint32 {
	return a.meta.GetPosition()
}

func (a *apiRuntime) matchRule() metapb.MatchRule {
	return a.meta.GetMatchRule()
}

func (v *apiValidation) validate(req *fasthttp.Request) bool {
	if len(v.rules) == 0 && !v.meta.Required {
		return true
	}

	value := paramValue(&v.meta.Parameter, req)
	if "" == value && v.meta.Required {
		return false
	} else if "" == value && !v.meta.Required {
		return true
	}

	for _, r := range v.rules {
		if !r.validate(hack.StringToSlice(value)) {
			return false
		}
	}

	return true
}

func (r *apiRule) validate(value []byte) bool {
	return r.pattern.Match(value)
}

type routingRuntime struct {
	meta    *metapb.Routing
	barrier *utils.RateBarrier
}

func newRoutingRuntime(meta *metapb.Routing) *routingRuntime {
	r := &routingRuntime{}
	r.updateMeta(meta)

	return r
}

func (a *routingRuntime) clone() *routingRuntime {
	meta := &metapb.Routing{}
	pbutil.MustUnmarshal(meta, pbutil.MustMarshal(a.meta))
	return newRoutingRuntime(meta)
}

func (a *routingRuntime) updateMeta(meta *metapb.Routing) {
	a.meta = meta
	a.barrier = utils.NewRateBarrier(int(a.meta.TrafficRate))
}

func (a *routingRuntime) matches(apiID uint64, req *fasthttp.Request, requestTag string) bool {
	if a.meta.API > 0 && apiID != a.meta.API {
		return false
	}

	for _, c := range a.meta.Conditions {
		if !conditionsMatches(&c, req) {
			log.Debugf("%s: skip routing %s by condition %+v",
				requestTag,
				a.meta.Name,
				c)
			return false
		}
	}

	value := a.barrier.Allow()
	if !value {
		log.Debugf("%s: skip routing %s by rate",
			requestTag,
			a.meta.Name)
	}

	return value
}

func (a *routingRuntime) isUp() bool {
	return a.meta.Status == metapb.Up
}

func conditionsMatches(cond *metapb.Condition, req *fasthttp.Request) bool {
	attrValue := paramValue(&cond.Parameter, req)
	if attrValue == "" {
		return false
	}

	switch cond.Cmp {
	case metapb.CMPEQ:
		return eq(attrValue, cond.Expect)
	case metapb.CMPLT:
		return lt(attrValue, cond.Expect)
	case metapb.CMPLE:
		return le(attrValue, cond.Expect)
	case metapb.CMPGT:
		return gt(attrValue, cond.Expect)
	case metapb.CMPGE:
		return ge(attrValue, cond.Expect)
	case metapb.CMPIn:
		return in(attrValue, cond.Expect)
	case metapb.CMPMatch:
		return reg(attrValue, cond.Expect)
	default:
		return false
	}
}

func eq(attrValue string, expect string) bool {
	return attrValue == expect
}

func lt(attrValue string, expect string) bool {
	s, err := strconv.Atoi(attrValue)
	if err != nil {
		return false
	}

	t, err := strconv.Atoi(expect)
	if err != nil {
		return false
	}

	return s < t
}

func le(attrValue string, expect string) bool {
	s, err := strconv.Atoi(attrValue)
	if err != nil {
		return false
	}

	t, err := strconv.Atoi(expect)
	if err != nil {
		return false
	}

	return s <= t
}

func gt(attrValue string, expect string) bool {
	s, err := strconv.Atoi(attrValue)
	if err != nil {
		return false
	}

	t, err := strconv.Atoi(expect)
	if err != nil {
		return false
	}

	return s > t
}

func ge(attrValue string, expect string) bool {
	s, err := strconv.Atoi(attrValue)
	if err != nil {
		return false
	}

	t, err := strconv.Atoi(expect)
	if err != nil {
		return false
	}

	return s >= t
}

func in(attrValue string, expect string) bool {
	return strings.Index(expect, attrValue) != -1
}

func reg(attrValue string, expect string) bool {
	matches, err := regexp.MatchString(expect, attrValue)
	return err == nil && matches
}

func paramValue(param *metapb.Parameter, req *fasthttp.Request) string {
	switch param.Source {
	case metapb.QueryString:
		return getQueryValue(param.Name, req)
	case metapb.FormData:
		return getFormValue(param.Name, req)
	case metapb.JSONBody:
		value, _, _, err := jsonparser.Get(req.Body(), param.Name)
		if err != nil {
			return ""
		}
		return hack.SliceToString(value)
	case metapb.Header:
		return getHeaderValue(param.Name, req)
	case metapb.Cookie:
		return getCookieValue(param.Name, req)
	case metapb.PathValue:
		return getPathValue(int(param.Index), req)
	default:
		return ""
	}
}

func getCookieValue(name string, req *fasthttp.Request) string {
	return hack.SliceToString(req.Header.Cookie(name))
}

func getHeaderValue(name string, req *fasthttp.Request) string {
	return hack.SliceToString(req.Header.Peek(name))
}

func getQueryValue(name string, req *fasthttp.Request) string {
	v, _ := url.QueryUnescape(hack.SliceToString(req.URI().QueryArgs().Peek(name)))
	return v
}

func getPathValue(idx int, req *fasthttp.Request) string {
	path := hack.SliceToString(req.URI().Path()[1:])
	values := strings.Split(path, "/")
	if len(values) <= idx {
		return ""
	}

	return values[idx]
}

func getFormValue(name string, req *fasthttp.Request) string {
	return string(req.PostArgs().Peek(name))
}
