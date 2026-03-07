package fakes

type FakePasswordHasher struct{}

func NewFakePasswordHasher() *FakePasswordHasher {
	return &FakePasswordHasher{}
}

func (h *FakePasswordHasher) Hash(password string) (string, error) {
	return "hashed:" + password, nil
}

func (h *FakePasswordHasher) Verify(password, hash string) (bool, error) {
	return hash == "hashed:"+password, nil
}
