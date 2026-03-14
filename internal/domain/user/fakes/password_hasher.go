package fakes

type FakePasswordHasher struct {
	Err error
}

func NewFakePasswordHasher() *FakePasswordHasher {
	return &FakePasswordHasher{}
}

func (h *FakePasswordHasher) Hash(password string) (string, error) {
	if h.Err != nil {
		return "", h.Err
	}
	return "hashed:" + password, nil
}

func (h *FakePasswordHasher) Verify(password, hash string) (bool, error) {
	if h.Err != nil {
		return false, h.Err
	}
	return hash == "hashed:"+password, nil
}
