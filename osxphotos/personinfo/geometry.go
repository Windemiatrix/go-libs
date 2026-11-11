package personinfo

import "math"

// fixOrientation is _fix_orientation (personinfo.py:372-410).
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

// Center is center (personinfo.py:214-221, 412-431): the center of the face in
// pixels, in PIL coordinates, truncated toward zero as Python int().
func (f FaceInfo) Center(width, height, orientation int) (x, y int) {
	fx, fy := fixOrientation(f.CenterX, f.CenterY, orientation)
	dx, dy := width, height
	if swapsAxes(orientation) {
		dx, dy = dy, dx
	}
	return int(fx * float64(dx)), int(fy * float64(dy))
}

// SizePixels is size_pixels (personinfo.py:223-232).
func (f FaceInfo) SizePixels(width, height int) float64 {
	return f.Size * float64(max(height, width))
}

// Rect is face_rect (personinfo.py:329-343): the corners x0, y0, x1, y1 of the
// square around the face.
func (f FaceInfo) Rect(width, height, orientation int) (x0, y0, x1, y1 float64) {
	radius := f.SizePixels(width, height) / 2
	cx, cy := f.Center(width, height, orientation)
	x, y := float64(cx), float64(cy)
	return x - radius, y - radius, x + radius, y + radius
}

// MWGArea is mwg_rs_area (personinfo.py:254-287). A photo with a zero width or
// height gives a zero Area (273-274). A pet with a zero Size and a body region
// gives the body region (_body_region, 200-212; 276-278).
func (f FaceInfo) MWGArea(width, height, orientation int) Area {
	if width == 0 || height == 0 {
		return Area{}
	}
	cx, cy := f.CenterX, f.CenterY
	s := f.SizePixels(width, height)
	h, w := s/float64(height), s/float64(width)
	if f.IsPet() && f.Size == 0 && f.BodyWidth != 0 && f.BodyHeight != 0 {
		cx, cy, h, w = f.BodyCenterX, f.BodyCenterY, f.BodyHeight, f.BodyWidth
	}
	x, y := fixOrientation(cx, cy, orientation)
	if swapsAxes(orientation) {
		h, w = w, h
	}
	return Area{x, y, h, w}
}

// MPRIRect is mpri_reg_rect (personinfo.py:289-327): X, Y is the top left
// corner; its H and W are those of mwg_rs_area exchanged and its corner is that
// center moved by half of them, as every branch of upstream computes (311-325).
// A photo with a zero width or height gives a zero Area (308-309).
func (f FaceInfo) MPRIRect(width, height, orientation int) Area {
	a := f.MWGArea(width, height, orientation)
	return Area{a.X - a.W/2, a.Y - a.H/2, a.W, a.H}
}

// RotatePoint is rotate_image_point (personinfo.py:520-546). The angle is in
// radians and positive counter-clockwise on a photo whose y axis points down.
func RotatePoint(x, y, xmid, ymid, angle float64) (xr, yr float64) {
	x -= xmid
	y -= ymid
	sin, cos := math.Sincos(angle)
	return x*cos + y*sin + xmid, -x*sin + y*cos + ymid
}
