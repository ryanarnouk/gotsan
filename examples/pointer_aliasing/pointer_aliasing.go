package multipointer

import (
	"fmt"
	"sync"
)

var mu1 sync.Mutex
var mu2 sync.Mutex

type Struct2 struct {
	// @guarded_by(mu2)
	value int
}

type Struct1 struct {
	// @guarded_by(mu1)
	test Struct2
}

func main() {
	example := Struct1{test: Struct2{value: 1}}

	fmt.Println(example.test.value)

	pointer1 := &example
	pointer2 := &pointer1
	pointer3 := &pointer2
	fmt.Println((***pointer3).test.value)
}
