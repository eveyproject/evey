package mkvgo

import (
	"bytes"
	"strconv"
	"strings"
)

// source_access.go - what mkvgo's Options.SourceAccess = SourceAuto decides for
// a library file: read as from a local disk (read-ahead asked for) or across a
// network (no read-ahead, audio reads sent together). mkvgo v0.52.0 exports no
// way to ask it, so its rule is copied here (mp4/prefetch_linux.go,
// mp4/mountinfo.go) for the startup log and the read-ratio journal
// (ondemand_readratio.go); Evey leaves SourceAuto as is.

// networkFUSE are the FUSE subtypes mkvgo takes as network mounts; any other
// FUSE mount, mergerfs first among them, is local.
var networkFUSE = map[string]bool{
	"rclone": true, "sshfs": true, "s3fs": true, "gcsfuse": true, "davfs": true, "davfs2": true,
	"goofys": true, "juicefs": true, "blobfuse": true, "blobfuse2": true, "ossfs": true, "curlftpfs": true,
	"smbnetfs": true, "httpfs2": true, "ceph-fuse": true, "glusterfs": true, "mfs": true, "lizardfs": true,
}

// networkStatfs are the statfs magics mkvgo takes as network mounts: NFS, SMB,
// CIFS, SMB2, 9p (WSL's drvfs among them), Ceph, AFS.
var networkStatfs = map[int64]bool{0x6969: true, 0x517B: true, 0xFF534D42: true, 0xFE534D42: true, 0x01021997: true, 0x00C36400: true, 0x5346414F: true}

const fuseMagic = 0x65735546

// SourceAccess is what SourceAuto decides for one file: Network, and the
// filesystem type mountinfo gives its device ("ext4", "fuse.mergerfs", "9p"...,
// empty when no line matches).
type SourceAccess struct {
	Network bool
	FSType  string
}

// Access is "network" or "local".
func (a SourceAccess) Access() string {
	if a.Network {
		return "network"
	}
	return "local"
}

// mountFSType is the filesystem type /proc/self/mountinfo gives the mount of
// device major:minor; empty when none matches.
func mountFSType(mountinfo []byte, major, minor uint64) string {
	dev := strconv.FormatUint(major, 10) + ":" + strconv.FormatUint(minor, 10)
	for line := range bytes.SplitSeq(mountinfo, []byte("\n")) {
		f := strings.Fields(string(line))
		if len(f) < 3 || f[2] != dev {
			continue
		}
		for i, x := range f {
			if x == "-" && i+1 < len(f) {
				return f[i+1]
			}
		}
	}
	return ""
}

// decideSourceAccess is mkvgo's rule over a file's statfs magic and its
// device's mountinfo type: a FUSE mount by its subtype, anything else by its
// magic.
func decideSourceAccess(magic int64, fsType string) bool {
	if magic == fuseMagic {
		sub, ok := strings.CutPrefix(fsType, "fuse.")
		return ok && networkFUSE[sub]
	}
	return networkStatfs[magic]
}

// sourceOnNetwork is what SourceAuto decides for path; a file that cannot be
// opened is a network source, as mkvgo takes it.
func sourceOnNetwork(path string) bool {
	a, err := SourceAccessOf(path)
	return err != nil || a.Network
}
