package fakes

type FakeFrontCompiler struct {
	Err error
}

func NewFakeFrontCompiler() *FakeFrontCompiler {
	return &FakeFrontCompiler{}
}

func (c *FakeFrontCompiler) CompileFront(_, _ string) error {
	return c.Err
}
