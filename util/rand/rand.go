package rand


import (
	"math/rand"
	"time"
)


// ----------------------------------------------------------------------------


func Uint64() uint64 {
	return rand.Uint64()
}


// ----------------------------------------------------------------------------


func init() {
	rand.Seed(time.Now().UnixNano())
}
