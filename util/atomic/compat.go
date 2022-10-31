package atomic


import (
	"sync"
)


// ----------------------------------------------------------------------------


type Int64 struct {
	lock sync.Mutex
	value int64
}

func (this *Int64) Add(val int64) int64 {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.value += val
	return this.value
}

func (this *Int64) Store(val int64) {
	this.lock.Lock()
	this.value = val
	this.lock.Unlock()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type Uint64 struct {
	lock sync.Mutex
	value uint64
}

func (this *Uint64) Add(val uint64) uint64 {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.value += val
	return this.value
}

func (this *Uint64) Store(val uint64) {
	this.lock.Lock()
	this.value = val
	this.lock.Unlock()
}


// ----------------------------------------------------------------------------
