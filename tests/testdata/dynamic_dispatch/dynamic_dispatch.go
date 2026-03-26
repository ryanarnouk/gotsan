package dynamicdispatch

type Doer interface {
	Do()
}

type worker struct{}

func (worker) Do() {}

func targetOne() {}

func callThroughParam(fn func()) {
	fn()
}

func triggerFunctionParam() {
	callThroughParam(targetOne)
}

func callThroughInterface(d Doer) {
	d.Do()
}

func triggerInterfaceParam() {
	var d Doer = worker{}
	callThroughInterface(d)
}

func runDoerInGoroutine(d Doer) {
	d.Do()
}

func triggerInterfaceParamViaGo() {
	var d Doer = worker{}
	go runDoerInGoroutine(d)
}

func dynamicTrampoline(fn func()) {
	fn()
}

func recursiveDriver() {
	dynamicTrampoline(recursiveDriver)
}
