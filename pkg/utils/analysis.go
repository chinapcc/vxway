package utils

import (
	"sync"
	"time"
	"vxway/pkg/atomic"
	"vxway/pkg/goetty"
	"vxway/pkg/log"
)

// Begin Point ////////////////////////////////////////////////////////////////////////////
type Point struct {
	requests          atomic.Int64
	rejects           atomic.Int64
	failure           atomic.Int64
	successed         atomic.Int64
	continuousFailure atomic.Int64

	costs atomic.Int64
	max   atomic.Int64
	min   atomic.Int64
}

func (p *Point) dump(target *Point) {
	target.requests.Set(p.requests.Get())
	target.rejects.Set(p.rejects.Get())
	target.failure.Set(p.failure.Get())
	target.successed.Set(p.successed.Get())
	target.max.Set(p.max.Get())
	target.min.Set(p.min.Get())
	target.costs.Set(p.costs.Get())
	p.min.Set(0)
	p.max.Set(0)
}

func newPoint() *Point {
	return &Point{}
}

// End Point ////////////////////////////////////////////////////////////////////////////

// Begin Analysis ////////////////////////////////////////////////////////////////////////////
// Analysis analysis struct
type Analysis struct {
	tw             *goetty.TimeoutWheel
	points         sync.Map //map[uint64]*point
	recentlyPoints sync.Map //map[uint64]map[time.Duration]*Recently
}

// RemoveTarget 移除某关键的分析点
func (a *Analysis) RemoveTarget(key uint64) {
	if v, ok := a.recentlyPoints.Load(key); ok {
		m := v.(*sync.Map)
		m.Range(func(key, value interface{}) bool {
			value.(*Recently).timeout.Stop()
			return true
		})
	}
	a.points.Delete(key)
	a.recentlyPoints.Delete(key)
}

// AddTarget 在键上添加分析点
func (a *Analysis) AddTarget(key uint64, interval time.Duration) {
	if interval == 0 {
		return
	}
	a.points.LoadOrStore(key, &Point{})
	a.recentlyPoints.LoadOrStore(key, &sync.Map{})
	v, _ := a.recentlyPoints.Load(key)
	vm := v.(*sync.Map)
	if _, ok := vm.Load(interval); ok {
		log.Info("analysis: already added, key=<%d> interval=<%s>",
			key,
			interval)
		return
	}

	recently := newRecently(key, interval)
	vm.Store(interval, recently)

	t, _ := a.tw.Schedule(interval, a.recentlyTimeout, recently)
	recently.timeout = t
	log.Info("analysis: added, key=<%d> interval=<%s>",
		key,
		interval)
}

// GetRecentlyRequestCount 返回指定持续时间中的服务器请求计数
func (a *Analysis) GetRecentlyRequestCount(server uint64, interval time.Duration) int {
	point := a.getPoint(server, interval)
	if point == nil {
		return 0
	}
	return int(point.requests)
}

// GetRecentlyMax 以指定秒为单位返回最大延迟时间
func (a *Analysis) GetRecentlyMax(server uint64, interval time.Duration) int {
	point := a.getPoint(server, interval)
	if point == nil {
		return 0
	}
	return int(point.max)
}

// GetRecentlyMin 返回指定持续时间的最小延迟
func (a *Analysis) GetRecentlyMin(server uint64, interval time.Duration) int {
	point := a.getPoint(server, interval)
	if point == nil {
		return 0
	}
	return int(point.min)
}

// GetRecentlyAvg 以指定秒为单位返回平均延迟
func (a *Analysis) GetRecentlyAvg(server uint64, interval time.Duration) int {
	point := a.getPoint(server, interval)
	if point == nil {
		return 0
	}
	return int(point.avg)
}

// GetQPS 返回规范持续时间的qps
func (a *Analysis) GetQPS(server uint64, interval time.Duration) int {
	point := a.getPoint(server, interval)
	if point == nil {
		return 0
	}
	return int(point.qps)
}

// GetRecentlyRejectCount 在规格持续时间中返回拒绝计数
func (a *Analysis) GetRecentlyRejectCount(server uint64, interval time.Duration) int {
	point := a.getPoint(server, interval)
	if point == nil {
		return 0
	}
	return int(point.rejects)
}

// GetRecentlyRequestSuccessedRate 以指定秒为单位返回成功率
func (a *Analysis) GetRecentlyRequestSuccessedRate(server uint64, interval time.Duration) int {
	point := a.getPoint(server, interval)
	if point == nil {
		return 0
	}
	if point.requests-point.rejects <= 0 {
		return 100
	}

	return int(point.successed * 100 / (point.requests - point.rejects))
}

// GetRecentlyRequestFailureRate 以指定秒为单位返回故障率
func (a *Analysis) GetRecentlyRequestFailureRate(server uint64, interval time.Duration) int {
	point := a.getPoint(server, interval)
	if point == nil {
		return 100
	}
	if point.requests-point.rejects <= 0 {
		return -1
	}
	return int(point.failure * 100 / (point.requests - point.rejects))
}

// GetRecentlyRequestSuccessedCount 以指定秒为单位返回成功的请求计数
func (a *Analysis) GetRecentlyRequestSuccessedCount(server uint64, interval time.Duration) int {
	point := a.getPoint(server, interval)
	if point == nil {
		return 0
	}
	return int(point.successed)
}

// GetRecentlyRequestFailureCount 在规定持续时间内返回失败请求数
func (a *Analysis) GetRecentlyRequestFailureCount(server uint64, interval time.Duration) int {
	point := a.getPoint(server, interval)
	if point == nil {
		return 0
	}
	return int(point.failure)
}

// GetContinuousFailureCount 以指定秒为单位返回连续失败请求计数
func (a *Analysis) GetContinuousFailureCount(key uint64) int {
	p, ok := a.points.Load(key)
	if !ok {
		return 0
	}
	return int(p.(*Point).continuousFailure.Get())
}

// Reject 增加拒绝数
func (a *Analysis) Reject(key uint64) {
	if p, ok := a.points.Load(key); ok {
		p.(*Point).rejects.Incr()
	}
}

// Failure 增加失败数
func (a *Analysis) Failure(key uint64) {
	if v, ok := a.points.Load(key); ok {
		p := v.(*Point)
		p.failure.Incr()
		p.continuousFailure.Incr()
	}
}

// Request 增加请求计数
func (a *Analysis) Request(key uint64) {
	if p, ok := a.points.Load(key); ok {
		p.(*Point).requests.Incr()
	}
}

// Response 增加成功数
func (a *Analysis) Response(key uint64, cost int64) {
	if v, ok := a.points.Load(key); ok {
		p := v.(*Point)
		p.successed.Incr()
		p.costs.Add(cost)
		p.continuousFailure.Set(0)

		if p.max.Get() < cost {
			p.max.Set(cost)
		}
		if p.min.Get() == 0 || p.min.Get() > cost {
			p.min.Set(cost)
		}
	}
}

func (a *Analysis) getPoint(key uint64, interval time.Duration) *Recently {
	points, ok := a.recentlyPoints.Load(key)
	if !ok {
		return nil
	}
	point, ok := points.(*sync.Map).Load(interval)
	if !ok {
		return nil
	}
	return point.(*Recently)
}

func (a *Analysis) recentlyTimeout(arg interface{}) {
	recently := arg.(*Recently)

	if p, ok := a.points.Load(recently.key); ok {
		recently.record(p.(*Point))
		t, _ := a.tw.Schedule(recently.period, a.recentlyTimeout, recently)
		recently.timeout = t
	}
}

// End Analysis ////////////////////////////////////////////////////////////////////////////

// Begin Recently ////////////////////////////////////////////////////////////////////////////
type Recently struct {
	key       uint64
	timeout   goetty.Timeout
	period    time.Duration
	prev      *Point
	current   *Point
	dumpPrev  bool
	qps       int
	requests  int64
	successed int64
	failure   int64
	rejects   int64
	max       int64
	min       int64
	avg       int64
}

func newRecently(key uint64, period time.Duration) *Recently {
	return &Recently{
		key:     key,
		prev:    newPoint(),
		current: newPoint(),
		period:  period,
	}
}

func (r *Recently) record(p *Point) {
	if !r.dumpPrev {
		p.dump(r.current)
		r.calc()
	} else {
		p.dump(r.prev)
	}
}

func (r *Recently) calc() {
	if r.current.requests.Get() == r.prev.requests.Get() {
		return
	}
	r.requests = r.current.requests.Get() - r.prev.requests.Get()
	if r.requests < 0 {
		r.requests = 0
	}
	r.successed = r.current.successed.Get() - r.prev.successed.Get()
	if r.successed < 0 {
		r.successed = 0
	}
	r.failure = r.current.failure.Get() - r.prev.failure.Get()
	if r.failure < 0 {
		r.failure = 0
	}
	r.rejects = r.current.rejects.Get() - r.prev.rejects.Get()
	if r.rejects < 0 {
		r.rejects = 0
	}
	r.max = r.current.max.Get()
	if r.max < 0 {
		r.max = 0
	} else {
		r.max = int64(r.max / 1000 / 1000)
	}
	r.min = r.current.min.Get()
	if r.min < 0 {
		r.min = 0
	} else {
		r.avg = int64(r.min / 1000 / 1000)
	}

	costs := r.current.costs.Get() - r.prev.costs.Get()
	if r.requests == 0 {
		r.avg = 0
	} else {
		r.avg = int64(costs / 1000 / 1000 / r.requests)
	}
	if r.successed > r.requests {
		r.qps = int(r.requests / int64(r.period/time.Second))
	} else {
		r.qps = int(r.successed / int64(r.period/time.Second))
	}
}

// End Analysis ////////////////////////////////////////////////////////////////////////////
