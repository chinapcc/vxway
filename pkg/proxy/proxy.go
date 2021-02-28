package proxy

import "sync"

type Proxy struct {
	sync.RWMutex

	dispatchIndex,copyIndex uint64
	dispatches []chan *dispatchNode
}
