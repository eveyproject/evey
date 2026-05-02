package ffmpeg

import (
	"image"
	"math"

	"golang.org/x/image/draw"
)

// pHash perceptual hash using DCT-II — produces a 64-bit signature far more
// robust to compression artifacts and minor scene changes than the average
// hash it replaces. The region [x0,y0,x1,y1] is scaled to 32×32 greyscale,
// 2D DCT-II is applied, the top-left 8×8 block is kept (low-frequency
// information), the DC coefficient (0,0) is dropped (depends on global
// luminance, hostile to comparisons across compression presets), and bit i
// of the output is set when AC coefficient i is greater than the median of
// the 63 remaining AC coefs.
//
// Distribution of Hamming distances on natural images:
//   identical frames        → 0
//   same shot, JPEG re-encode → 0–4
//   same intro across episodes → 4–14
//   unrelated scenes          → ~32 (random)
//
// The pipeline's max_hamming default of 8 still works — pHash distribution
// is tighter than aHash for "same intro" matches but slightly looser for
// truly identical frames; cf. bench dataset.
func pHashDCT(img image.Image, x0, y0, x1, y1 int) uint64 {
	const inSize = 32
	gray := image.NewGray(image.Rect(0, 0, inSize, inSize))
	src := img

	srcBounds := image.Rect(x0, y0, x1, y1)
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	if si, ok := src.(subImager); ok {
		src = si.SubImage(srcBounds)
	}

	draw.BiLinear.Scale(gray, gray.Bounds(), src, srcBounds, draw.Src, nil)

	// Copy into float64 grid for the DCT.
	var grid [inSize][inSize]float64
	for y := 0; y < inSize; y++ {
		for x := 0; x < inSize; x++ {
			grid[y][x] = float64(gray.Pix[y*gray.Stride+x])
		}
	}

	dctRows := dct1D32Rows(grid)
	dctFull := dct1D32Cols(dctRows)

	// Take the top-left 8×8 (low-frequency) block.
	const outSize = 8
	var coefs [outSize * outSize]float64
	for y := 0; y < outSize; y++ {
		for x := 0; x < outSize; x++ {
			coefs[y*outSize+x] = dctFull[y][x]
		}
	}

	// Drop DC term (index 0) and compute median of the 63 AC terms.
	median := medianFloat63(coefs[1:])

	var h uint64
	// Bit 0 stays unused (DC dropped); bits 1..63 cover AC terms in
	// row-major order. This is intentional: it keeps the bit layout
	// trivially mappable back to (y,x) when debugging without any
	// special-case handling for skipping index 0.
	for i := 1; i < outSize*outSize; i++ {
		if coefs[i] > median {
			h |= 1 << uint(i)
		}
	}
	return h
}

// dct1D32Rows applies a 1D DCT-II to every row of a 32×32 grid.
func dct1D32Rows(in [32][32]float64) [32][32]float64 {
	var out [32][32]float64
	for r := 0; r < 32; r++ {
		for k := 0; k < 32; k++ {
			var sum float64
			for n := 0; n < 32; n++ {
				sum += in[r][n] * dctCos32[n][k]
			}
			out[r][k] = sum
		}
	}
	return out
}

// dct1D32Cols applies a 1D DCT-II to every column of a 32×32 grid.
func dct1D32Cols(in [32][32]float64) [32][32]float64 {
	var out [32][32]float64
	for c := 0; c < 32; c++ {
		for k := 0; k < 32; k++ {
			var sum float64
			for n := 0; n < 32; n++ {
				sum += in[n][c] * dctCos32[n][k]
			}
			out[k][c] = sum
		}
	}
	return out
}

// medianFloat63 returns the median of a slice of 63 floats. Uses an
// insertion sort on a stack-allocated copy — fine for the fixed small size.
func medianFloat63(values []float64) float64 {
	var buf [63]float64
	copy(buf[:], values)
	for i := 1; i < len(buf); i++ {
		for j := i; j > 0 && buf[j] < buf[j-1]; j-- {
			buf[j], buf[j-1] = buf[j-1], buf[j]
		}
	}
	return buf[len(buf)/2]
}

// dctCos32[n][k] = cos((π/N) * (n+0.5) * k) with N=32. Pre-computed once at
// init() time — saves 1024 cos() calls per pHash and matters since each
// trickplay sprite holds 25 tiles and a series can have hundreds of sprites.
var dctCos32 [32][32]float64

func init() {
	const N = 32
	for n := 0; n < N; n++ {
		for k := 0; k < N; k++ {
			dctCos32[n][k] = math.Cos((math.Pi / N) * (float64(n) + 0.5) * float64(k))
		}
	}
}
