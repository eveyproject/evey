package offlineaudio

import (
	"github.com/gravity-zero/ac3go/ac3"
	"github.com/gravity-zero/ac3go/pcm"
)

// FrameDecoder is ac3go as the player's audio decoder: the Kotlin renderer
// hands it one access unit per call (an AC-3 syncframe, or an E-AC-3 sample
// that can hold several syncframes) and gets interleaved s16le back, the
// format the audio sink and the preamp take. Not safe for concurrent use.
//
// The output layout is fixed by the first frame, since the audio sink is
// configured once: stereo when the output takes at most two channels or the
// stream codes no more, else 5.1 in Android's order, or 7.1 for an E-AC-3
// 7.1 stream on an output that takes eight.
type FrameDecoder struct {
	dec    *ac3.Decoder
	maxCh  int
	outCh  int // 0 until the first frame decides
	out    []byte
	rate   int
	bad    int64
	mixFor pcm.Layout // source layout the mix below was built for
	mix    [][]mixTap // per output channel, the source planes summed into it
}

type mixTap struct {
	src  int
	gain float32
}

// Android's PCM channel orders (AudioFormat.CHANNEL_OUT_5POINT1 and
// CHANNEL_OUT_7POINT1_SURROUND); ac3go's 7.1 layout is already the latter.
var (
	android51 = pcm.Layout{pcm.ChannelLeft, pcm.ChannelRight, pcm.ChannelCenter, pcm.ChannelLFE, pcm.ChannelBackLeft, pcm.ChannelBackRight}
	android71 = pcm.Layout7point1
)

const minus3dB = 0.70710677

// NewFrameDecoder returns a decoder for an output taking up to maxChannels.
func NewFrameDecoder(maxChannels int) (*FrameDecoder, error) {
	return &FrameDecoder{dec: ac3.NewDecoder(), maxCh: maxChannels}, nil
}

// Decode returns the interleaved s16le PCM of every independent syncframe in
// au. A frame that fails to decode yields silence of its own length so the
// audio clock never drifts from the video; bytes that do not parse as a
// syncframe end the unit. Empty output means nothing was usable.
func (d *FrameDecoder) Decode(au []byte) ([]byte, error) {
	d.out = d.out[:0]
	for len(au) > 0 {
		var hdr ac3.Header
		if ac3.ParseHeader(au, &hdr) != nil {
			break
		}
		size := hdr.Sync.FrameSize
		if size <= 0 || size > len(au) {
			break
		}
		// Dependent substreams only add 7.1 channels: the multichannel path
		// merges them with their core below, stereo drops them like the
		// offline pass does (see Session.decodeNext).
		if hdr.Sync.Strmtyp == ac3.StrmtypDependent || hdr.Sync.Substreamid != 0 {
			au = au[size:]
			continue
		}
		if d.outCh == 0 && !d.chooseStereo(&hdr) {
			d.outCh = -1 // multichannel, the count is known after the decode
		}
		frame := au
		if d.outCh == 2 {
			frame = au[:size]
		}
		n := hdr.Sync.NumBlocks * ac3.SamplesPerBlock
		if err := d.dec.DecodeFrame(frame); err != nil {
			au = au[size:]
			d.bad++
			// The filter bank's carry over is garbage after a failed frame.
			d.dec.Reset()
			if d.outCh > 0 {
				d.out = append(d.out, make([]byte, n*d.outCh*2)...)
			}
			continue
		}
		au = au[d.dec.AccessUnitSize():]
		if d.outCh == -1 {
			d.outCh = 6
			if d.maxCh >= 8 && d.dec.OutputChannels() == 8 {
				d.outCh = 8
			}
		}
		if d.rate == 0 {
			d.rate = hdr.Sync.SampleRate
		}
		if d.outCh == 2 {
			d.out = appendStereoS16(d.out, d.dec.Samples(0)[:n], d.dec.Samples(1)[:n])
		} else {
			d.appendMultichannel(n)
		}
	}
	return d.out, nil
}

// chooseStereo fixes the output at stereo, with ac3go's downmix, when the
// output or the stream has no more than two channels.
func (d *FrameDecoder) chooseStereo(hdr *ac3.Header) bool {
	if d.maxCh >= 6 && hdr.Channels() > 2 {
		return false
	}
	d.outCh = 2
	return d.dec.SetDownmix(pcm.LayoutStereo) == nil
}

// appendMultichannel maps the decoded planes onto the output layout and
// appends them interleaved as s16le.
func (d *FrameDecoder) appendMultichannel(n int) {
	src := d.dec.OutputLayout()
	if !sameLayout(src, d.mixFor) {
		d.mix = buildMix(src, d.outCh)
		d.mixFor = append(d.mixFor[:0], src...)
	}
	planes := make([][]float32, len(src))
	for i := range src {
		planes[i] = d.dec.Samples(i)[:n]
	}
	base := len(d.out)
	d.out = append(d.out, make([]byte, n*d.outCh*2)...)
	out := d.out[base:]
	for i := range n {
		for ch, taps := range d.mix {
			var v float32
			for _, t := range taps {
				v += planes[t.src][i] * t.gain
			}
			s := s16le(v)
			o := (i*d.outCh + ch) * 2
			out[o] = byte(s)
			out[o+1] = byte(s >> 8)
		}
	}
}

// buildMix routes each source channel to its speaker in Android's order. A
// mono surround feeds both back speakers at -3 dB, and on a 5.1 output the
// side and back pairs of a 7.1 source share the back speakers the same way.
func buildMix(src pcm.Layout, outCh int) [][]mixTap {
	target := android51
	if outCh == 8 {
		target = android71
	}
	mix := make([][]mixTap, len(target))
	route := func(to pcm.Channel, from int, gain float32) {
		if i := target.Index(to); i >= 0 {
			mix[i] = append(mix[i], mixTap{from, gain})
		}
	}
	has71 := src.Has(pcm.ChannelSideLeft) && outCh == 6
	for i, c := range src {
		switch c {
		case pcm.ChannelCh1:
			route(pcm.ChannelLeft, i, 1)
		case pcm.ChannelCh2:
			route(pcm.ChannelRight, i, 1)
		case pcm.ChannelLeftSurround:
			route(pcm.ChannelBackLeft, i, 1)
		case pcm.ChannelRightSurround:
			route(pcm.ChannelBackRight, i, 1)
		case pcm.ChannelMonoSurround:
			route(pcm.ChannelBackLeft, i, minus3dB)
			route(pcm.ChannelBackRight, i, minus3dB)
		case pcm.ChannelSideLeft, pcm.ChannelBackLeft:
			if has71 {
				route(pcm.ChannelBackLeft, i, minus3dB)
			} else {
				route(c, i, 1)
			}
		case pcm.ChannelSideRight, pcm.ChannelBackRight:
			if has71 {
				route(pcm.ChannelBackRight, i, minus3dB)
			} else {
				route(c, i, 1)
			}
		default:
			route(c, i, 1)
		}
	}
	return mix
}

func sameLayout(a, b pcm.Layout) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Channels is the output's channel count, 0 until a frame has decoded.
func (d *FrameDecoder) Channels() int {
	if d.outCh < 0 {
		return 0
	}
	return d.outCh
}

// SampleRate is the stream's rate in Hz, 0 until a frame has decoded.
func (d *FrameDecoder) SampleRate() int { return d.rate }

// BadFrames counts the frames replaced by silence since creation.
func (d *FrameDecoder) BadFrames() int64 { return d.bad }

// Reset drops the state carried between frames, for a seek or a flush.
func (d *FrameDecoder) Reset() { d.dec.Reset() }
