package personinfo

import "math"

// fixOrientation is _fix_orientation (personinfo.py:337-375).
func fixOrientation(x, y float64, orientation int) (fx, fy float64) {
	switch orientation {
	case 0, 1:
		return x, 1 - y
	case 2:
		return 1 - x, 1 - y
	case 3:
		return 1 - x, y
	case 5:
		return 1 - y, x
	case 6:
		return 1 - y, 1 - x
	case 7:
		return y, 1 - x
	case 8:
		return y, x
	}
	return x, y
}

func swapsAxes(orientation int) bool { return orientation >= 5 && orientation <= 8 }

// Center is center (personinfo.py:188-195, 377-396): the center of the face in
// pixels, in PIL coordinates, truncated toward zero as Python int().
func (f FaceInfo) Center(width, height, orientation int) (x, y int) {
	fx, fy := fixOrientation(f.CenterX, f.CenterY, orientation)
	dx, dy := width, height
	if swapsAxes(orientation) {
		dx, dy = dy, dx
	}
	return int(fx * float64(dx)), int(fy * float64(dy))
}

// SizePixels is size_pixels (personinfo.py:197-206).
func (f FaceInfo) SizePixels(width, height int) float64 {
	return f.Size * float64(max(height, width))
}

// Rect is face_rect (personinfo.py:294-308): the corners x0, y0, x1, y1 of the
// square around the face.
func (f FaceInfo) Rect(width, height, orientation int) (x0, y0, x1, y1 float64) {
	radius := f.SizePixels(width, height) / 2
	cx, cy := f.Center(width, height, orientation)
	x, y := float64(cx), float64(cy)
	return x - radius, y - radius, x + radius, y + radius
}

// MWGArea is mwg_rs_area (personinfo.py:228-257). A photo with a zero width or
// height gives a zero Area (247-248).
func (f FaceInfo) MWGArea(width, height, orientation int) Area {
	if width == 0 || height == 0 {
		return Area{}
	}
	x, y := fixOrientation(f.CenterX, f.CenterY, orientation)
	s := f.SizePixels(width, height)
	h, w := s/float64(height), s/float64(width)
	if swapsAxes(orientation) {
		h, w = w, h
	}
	return Area{x, y, h, w}
}

// MPRIRect is mpri_reg_rect (personinfo.py:259-292): X, Y is the top left
// corner; its H and W are those of mwg_rs_area exchanged and its corner is that
// center moved by half of them, as both branches of upstream compute (281-290).
// A photo with a zero width or height gives a zero Area (278-279).
func (f FaceInfo) MPRIRect(width, height, orientation int) Area {
	a := f.MWGArea(width, height, orientation)
	return Area{a.X - a.W/2, a.Y - a.H/2, a.W, a.H}
}

// RotatePoint is rotate_image_point (personinfo.py:479-505). The angle is in
// radians and positive counter-clockwise on a photo whose y axis points down.
func RotatePoint(x, y, xmid, ymid, angle float64) (xr, yr float64) {
	x -= xmid
	y -= ymid
	sin, cos := math.Sincos(angle)
	return x*cos + y*sin + xmid, -x*sin + y*cos + ymid
}
