package fakes

import "sync"

type FakePasswordHasher struct {
	Err error
	// mu guards the call-tracking fields below: Verify is invoked concurrently
	// by the concurrent-login tests, so the counter/slice must be synchronized
	// or -race trips.
	mu sync.Mutex
	// VerifyCalls counts Verify invocations and VerifiedHashes records the hash
	// argument of each, so tests can assert the constant-time dummy-verify path
	// runs even when the username is unknown (username-enumeration defense).
	VerifyCalls    int
	VerifiedHashes []string
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
	h.mu.Lock()
	h.VerifyCalls++
	h.VerifiedHashes = append(h.VerifiedHashes, hash)
	h.mu.Unlock()
	if h.Err != nil {
		return false, h.Err
	}
	return hash == "hashed:"+password, nil
}
