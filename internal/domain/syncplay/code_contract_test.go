package syncplay

import (
	"os"
	"regexp"
	"testing"
)

// Cross-language drift guard. The server MINTS room codes and four client
// surfaces let a guest enter one; if the alphabets or the lengths ever
// disagree, every guest join fails as a plain "room not found" - which reads as
// a server problem, not a client one, and nothing else pins the two together.
//
// It lives on the Go side because the frontend suites run in a container where
// only frontend/ is mounted, so the mirror test could not read this file.
const clientRoomCodeSrc = "../../../frontend/packages/api/src/roomCode.ts"

func TestRoomCodeContractMatchesTheClients(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(clientRoomCodeSrc)
	if err != nil {
		t.Fatalf("read %s: %v", clientRoomCodeSrc, err)
	}

	alphabet := regexp.MustCompile(`ROOM_CODE_ALPHABET = "([^"]+)"`).FindSubmatch(src)
	if alphabet == nil {
		t.Fatal("ROOM_CODE_ALPHABET not found - was it renamed client-side?")
	}
	if got := string(alphabet[1]); got != codeAlphabet {
		t.Errorf("client alphabet = %q, server = %q", got, codeAlphabet)
	}

	length := regexp.MustCompile(`ROOM_CODE_LENGTH = (\d+)`).FindSubmatch(src)
	if length == nil {
		t.Fatal("ROOM_CODE_LENGTH not found - was it renamed client-side?")
	}
	if got := string(length[1]); got != "6" || codeLen != 6 {
		t.Errorf("client length = %s, server = %d", got, codeLen)
	}
}
