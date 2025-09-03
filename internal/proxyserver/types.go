package proxyserver

type RunOpts struct {
	VncPort      int
	Port         int
	StrictAuth   bool
	NoVncVersion string
}
