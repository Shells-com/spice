package spice

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
)

type Rect struct {
	Top    uint32
	Left   uint32
	Bottom uint32
	Right  uint32
}

func (rect *Rect) Decode(r *bytes.Reader) error {
	binary.Read(r, binary.LittleEndian, &rect.Top)
	binary.Read(r, binary.LittleEndian, &rect.Left)
	binary.Read(r, binary.LittleEndian, &rect.Bottom)
	return binary.Read(r, binary.LittleEndian, &rect.Right)
}

func (rect *Rect) Rectangle() image.Rectangle {
	return image.Rectangle{
		Min: image.Point{X: int(rect.Left), Y: int(rect.Top)},
		Max: image.Point{X: int(rect.Right), Y: int(rect.Bottom)},
	}
}

type Point struct {
	X uint32
	Y uint32
}

func (point *Point) Decode(r *bytes.Reader) error {
	if err := binary.Read(r, binary.LittleEndian, &point.X); err != nil {
		return err
	}
	return binary.Read(r, binary.LittleEndian, &point.Y)
}

type QMask struct {
	MaskFlags uint8 // 1=INVERS
	Pos       Point
	ImagePtr  uint32
	*Image
}

func (qmask *QMask) Decode(r *bytes.Reader) error {
	binary.Read(r, binary.LittleEndian, &qmask.MaskFlags)
	qmask.Pos.Decode(r)
	return binary.Read(r, binary.LittleEndian, &qmask.ImagePtr)
}

type DisplayBase struct {
	Surface  uint32
	Box      Rect   // Rect box
	ClipType uint8  // 0=NONE, 1=RECTS
	NumRects uint32 // if clip_type=1
	Rects    []Rect // if clip_type=1
}

func (res *DisplayBase) Decode(r *bytes.Reader) error {
	binary.Read(r, binary.LittleEndian, &res.Surface)
	res.Box.Decode(r)
	binary.Read(r, binary.LittleEndian, &res.ClipType)

	switch res.ClipType {
	case 0:
		return nil
	case 1:
		binary.Read(r, binary.LittleEndian, &res.NumRects)

		res.Rects = make([]Rect, res.NumRects)
		for i := uint32(0); i < res.NumRects; i++ {
			res.Rects[i].Decode(r)
		}
		return nil
	default:
		return errors.New("invalid display base")
	}
}
