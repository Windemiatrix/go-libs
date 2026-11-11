package personinfo

// AsDict is asdict of PersonInfo (personinfo.py:112-125). uuid, displayname
// and keyphoto are nil for "" (None upstream); name is as held, never None
// upstream (photosdb.py:1796-1800).
func (p PersonInfo) AsDict() map[string]any {
	return map[string]any{
		"uuid": strOrNil(p.UUID), "name": p.Name, "displayname": strOrNil(p.DisplayName), "keyface": p.KeyFace,
		"facecount": p.FaceCount, "keyphoto": strOrNil(p.KeyPhotoUUID), "favorite": p.Favorite, "sort_order": p.SortOrder,
		"feature_less": p.FeatureLess,
	}
}

// AsDict is asdict of FaceInfo (personinfo.py:455-496) for a photo of width,
// height and orientation, as Center takes them.
// center is a pair of int, face_rect two pairs of float64.
// mwg_rs_area and mpri_reg_rect are maps of x, y, h and w, integer zeros for a
// photo of no width or height (273-274, 308-309).
// has_smile, manual and intrash are 1 or 0, the integer column as upstream
// keeps it, 0 for NULL where upstream has None.
// detection_type and body_* are 0 for NULL, where upstream has None.
// face_type is nil, pitch and yaw 0, as upstream for Photos 5 and later
// (_photosdb_process_faceinfo.py:323, 341-344).
// roll is 0.0 for a NULL ZROLL, where upstream has the integer 0
// (personinfo.py:348).
// uuid, name and asset_uuid are nil for "" (None upstream).
func (f FaceInfo) AsDict(width, height, orientation int) map[string]any {
	cx, cy := f.Center(width, height, orientation)
	x0, y0, x1, y1 := f.Rect(width, height, orientation)
	return map[string]any{
		"_pk": f.PK, "uuid": strOrNil(f.UUID), "name": strOrNil(f.Name), "asset_uuid": strOrNil(f.AssetUUID), "_person_pk": f.PersonPK,
		"center_x": f.CenterX, "center_y": f.CenterY, "center": []any{cx, cy}, "size": f.Size, "face_rect": []any{[]any{x0, y0}, []any{x1, y1}},
		"mpri_reg_rect": areaDict(f.MPRIRect(width, height, orientation), width, height), "roll": f.Roll, "pitch": 0, "yaw": 0,
		"mwg_rs_area": areaDict(f.MWGArea(width, height, orientation), width, height), "quality": f.Quality,
		"source_width": f.SourceWidth, "source_height": f.SourceHeight, "has_smile": bit(f.HasSmile), "manual": bit(f.Manual),
		"face_type": nil, "age_type": f.AgeType, "eye_makeup_type": f.EyeMakeupType, "eye_state": f.EyeState,
		"facial_hair_type": f.FacialHairType, "gender_type": f.GenderType, "glasses_type": f.GlassesType,
		"hair_color_type": f.HairColorType, "intrash": bit(f.InTrash), "lip_makeup_type": f.LipMakeupType,
		"smile_type": f.SmileType, "detection_type": f.DetectionType, "is_pet": f.IsPet(),
		"body_center_x": f.BodyCenterX, "body_center_y": f.BodyCenterY, "body_width": f.BodyWidth, "body_height": f.BodyHeight,
	}
}

func areaDict(a Area, width, height int) map[string]any {
	if width == 0 || height == 0 {
		return map[string]any{"x": 0, "y": 0, "h": 0, "w": 0}
	}
	return map[string]any{"x": a.X, "y": a.Y, "h": a.H, "w": a.W}
}

func bit(b bool) int {
	if b {
		return 1
	}
	return 0
}

// PersonInfo and FaceInfo hold "" where upstream has None, which json writes
// as null.
func strOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}
