// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/maloquacious/tecto"
)

// The heights file is a header and then one height for each hex. Every
// number is little-endian.
//
//	offset  size  what
//	0       8     the bytes "TECTOHGT"
//	8       4     format version, an unsigned integer: 1
//	12      4     columns, an unsigned integer
//	16      4     rows, an unsigned integer
//	20      4     detail, an unsigned integer
//	24      8     seed, an unsigned integer
//	32      8     simulated time in millions of years, a 64-bit float
//	40      4×n   heights in metres above sea level, 32-bit floats,
//	              n = columns × rows, hex (column, row) at row×columns + column
const (
	fileMagic   = "TECTOHGT"
	fileVersion = 1
	headerSize  = 40
)

// encodeHeights returns the heights file for a result and the parameters
// that gave it. The parameters must have their defaults filled in.
func encodeHeights(p tecto.Params, r *tecto.Result) []byte {
	le := binary.LittleEndian
	buf := make([]byte, 0, headerSize+4*len(r.Heights))
	buf = append(buf, fileMagic...)
	buf = le.AppendUint32(buf, fileVersion)
	buf = le.AppendUint32(buf, uint32(r.Columns))
	buf = le.AppendUint32(buf, uint32(r.Rows))
	buf = le.AppendUint32(buf, uint32(p.Detail))
	buf = le.AppendUint64(buf, p.Seed)
	buf = le.AppendUint64(buf, math.Float64bits(p.Myr))
	for _, h := range r.Heights {
		buf = le.AppendUint32(buf, math.Float32bits(h))
	}
	return buf
}

// A color at a height, in metres above sea level.
type stop struct {
	height float64
	color  [3]float64
}

// The picture's colors, from the deepest sea to the highest ground. The
// color between two stops is a blend of them. Sea level has two stops, so
// the coast is a sharp line.
var stops = []stop{
	{-6000, [3]float64{8, 20, 60}},
	{-3000, [3]float64{20, 60, 130}},
	{0, [3]float64{130, 190, 230}},
	{0, [3]float64{60, 130, 70}},
	{1000, [3]float64{150, 180, 100}},
	{2500, [3]float64{170, 130, 80}},
	{4000, [3]float64{120, 90, 70}},
	{6000, [3]float64{250, 250, 250}},
}

func colorAt(height float64) color.NRGBA {
	c := stops[len(stops)-1].color
	switch {
	case height <= stops[0].height:
		c = stops[0].color
	default:
		for i := 1; i < len(stops); i++ {
			if a, b := stops[i-1], stops[i]; height <= b.height && b.height > a.height {
				t := (height - a.height) / (b.height - a.height)
				for k := range c {
					c[k] = a.color[k] + t*(b.color[k]-a.color[k])
				}
				break
			}
		}
	}
	return color.NRGBA{R: uint8(c[0]), G: uint8(c[1]), B: uint8(c[2]), A: 255}
}

// encodePNG returns a picture of the heights, one pixel for each hex. It is
// for looking at, not measuring: it ignores the half-row stagger of the odd
// columns, and a hex is not as wide as it is tall.
func encodePNG(r *tecto.Result) ([]byte, error) {
	img := image.NewNRGBA(image.Rect(0, 0, r.Columns, r.Rows))
	for row := range r.Rows {
		for col := range r.Columns {
			img.SetNRGBA(col, row, colorAt(float64(r.Heights[row*r.Columns+col])))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
