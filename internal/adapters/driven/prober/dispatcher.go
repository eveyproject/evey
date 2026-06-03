package prober

import (
	"context"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"

	mkvadapter "github.com/evey-project/evey/internal/adapters/driven/mkvgo"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// mkvMetaReader is the slice of mkvgo the dispatcher depends on (so tests can
// inject a fake without touching the filesystem or the real library).
type mkvMetaReader interface {
	IsEBML(path string) bool
	ReadMeta(ctx context.Context, path string) (*extmkv.Container, error)
}

// Dispatcher implements library.StreamProber. For an EBML container (.mkv/.webm
// family) it reads metadata in-process via mkvgo ReadMeta and maps it to a
// ProbeResult; for everything else — and for any .mkv it cannot map
// field-equivalent — it delegates to fallback (the ffprobe prober, the
// authority). The fast-path is a fork-free, head-only optimization, never a
// correctness requirement.
type Dispatcher struct {
	fallback library.StreamProber
	mkv      mkvMetaReader
	// enabled gates the fast-path at runtime (nil == always on). Wired to a
	// config flag so the in-process path can be toggled off instantly without a
	// rebuild; when it returns false the dispatcher is a pure pass-through.
	enabled func() bool
}

var _ library.StreamProber = (*Dispatcher)(nil)

// New builds a Dispatcher over the ffprobe fallback, using the real mkvgo
// reader. enabled gates the fast-path (nil == always on).
func New(fallback library.StreamProber, enabled func() bool) *Dispatcher {
	return &Dispatcher{fallback: fallback, mkv: realMKVReader{}, enabled: enabled}
}

func (d *Dispatcher) ProbeStreams(ctx context.Context, filePath string) (*library.ProbeResult, error) {
	ctx, span := tracing.StartSpan(ctx, "prober.dispatch")
	defer span.End()

	if (d.enabled != nil && !d.enabled()) || d.mkv == nil || !d.mkv.IsEBML(filePath) {
		return d.fallback.ProbeStreams(ctx, filePath)
	}
	c, err := d.mkv.ReadMeta(ctx, filePath)
	if err != nil {
		span.AddEvent("mkv-readmeta-error: ffprobe fallback")
		return d.fallback.ProbeStreams(ctx, filePath)
	}
	res, ok := mapContainer(c)
	if !ok {
		span.AddEvent("mkv-unmappable: ffprobe fallback")
		return d.fallback.ProbeStreams(ctx, filePath)
	}
	span.AddEvent("mkv-fastpath")
	return res, nil
}

// realMKVReader is the production mkvMetaReader backed by the mkvgo library.
type realMKVReader struct{}

func (realMKVReader) IsEBML(path string) bool { return mkvadapter.IsEBMLContainer(path) }

func (realMKVReader) ReadMeta(ctx context.Context, path string) (*extmkv.Container, error) {
	return reader.OpenMeta(ctx, path)
}
