package fakes

type FakeWidgetCompiler struct {
	Err error
}

func NewFakeWidgetCompiler() *FakeWidgetCompiler {
	return &FakeWidgetCompiler{}
}

func (c *FakeWidgetCompiler) CompileWidget(_, _ string) error {
	return c.Err
}
