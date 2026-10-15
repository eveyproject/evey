//go:build linux && (amd64 || arm64)

package mkvgo

import (
	"os"
	"syscall"
)

// SourceAccessOf opens path and says what mkvgo's SourceAuto decides for it. A
// failing Fstat or Fstatfs is a network source, as in mkvgo.
func SourceAccessOf(path string) (SourceAccess, error) {
	f, err := os.Open(path)
	if err != nil {
		return SourceAccess{}, err
	}
	defer f.Close()
	var st syscall.Stat_t
	if err := syscall.Fstat(int(f.Fd()), &st); err != nil {
		return SourceAccess{Network: true}, nil
	}
	major := (st.Dev>>8)&0xfff | (st.Dev>>32)&^0xfff
	minor := st.Dev&0xff | (st.Dev>>12)&^0xff
	fsType := ""
	if mi, err := os.ReadFile("/proc/self/mountinfo"); err == nil {
		fsType = mountFSType(mi, major, minor)
	}
	var sfs syscall.Statfs_t
	if err := syscall.Fstatfs(int(f.Fd()), &sfs); err != nil {
		return SourceAccess{Network: true, FSType: fsType}, nil
	}
	return SourceAccess{Network: decideSourceAccess(sfs.Type, fsType), FSType: fsType}, nil
}
