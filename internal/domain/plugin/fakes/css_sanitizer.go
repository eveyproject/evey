package fakes

type FakeCSSSanitizer struct {
	Output   []byte
	Warnings []string
}

func NewFakeCSSSanitizer() *FakeCSSSanitizer {
	return &FakeCSSSanitizer{}
}

func (f *FakeCSSSanitizer) SanitizeCSS(input []byte) ([]byte, []string) {
	if f.Output != nil {
		return f.Output, f.Warnings
	}
	return input, f.Warnings
}
