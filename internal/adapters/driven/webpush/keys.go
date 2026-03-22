package webpush

import wp "github.com/SherClockHolmes/webpush-go"

func GenerateVAPIDKeys() (privateKey, publicKey string, err error) {
	return wp.GenerateVAPIDKeys()
}
