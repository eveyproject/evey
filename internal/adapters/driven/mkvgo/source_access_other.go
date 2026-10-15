//go:build !(linux && (amd64 || arm64))

package mkvgo

import "os"

// SourceAccessOf: outside Linux amd64/arm64 mkvgo always reads as local.
func SourceAccessOf(path string) (SourceAccess, error) {
	f, err := os.Open(path)
	if err != nil {
		return SourceAccess{}, err
	}
	_ = f.Close()
	return SourceAccess{}, nil
}
