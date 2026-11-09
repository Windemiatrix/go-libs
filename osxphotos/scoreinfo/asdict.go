package scoreinfo

// AsDict is asdict of ScoreInfo (scoreinfo.py:42-44): each score a float64,
// nil for a nil score, as None upstream. The map is new on every call.
func (s ScoreInfo) AsDict() map[string]any {
	return map[string]any{
		"overall": orNil(s.Overall), "curation": orNil(s.Curation), "promotion": orNil(s.Promotion),
		"highlight_visibility": orNil(s.HighlightVisibility), "behavioral": orNil(s.Behavioral), "failure": orNil(s.Failure),
		"harmonious_color": orNil(s.HarmoniousColor), "immersiveness": orNil(s.Immersiveness), "interaction": orNil(s.Interaction),
		"interesting_subject": orNil(s.InterestingSubject), "intrusive_object_presence": orNil(s.IntrusiveObjectPresence),
		"lively_color": orNil(s.LivelyColor), "low_light": orNil(s.LowLight), "noise": orNil(s.Noise),
		"pleasant_camera_tilt": orNil(s.PleasantCameraTilt), "pleasant_composition": orNil(s.PleasantComposition),
		"pleasant_lighting": orNil(s.PleasantLighting), "pleasant_pattern": orNil(s.PleasantPattern),
		"pleasant_perspective": orNil(s.PleasantPerspective), "pleasant_post_processing": orNil(s.PleasantPostProcessing),
		"pleasant_reflection": orNil(s.PleasantReflection), "pleasant_symmetry": orNil(s.PleasantSymmetry),
		"sharply_focused_subject": orNil(s.SharplyFocusedSubject), "tastefully_blurred": orNil(s.TastefullyBlurred),
		"well_chosen_subject": orNil(s.WellChosenSubject), "well_framed_subject": orNil(s.WellFramedSubject),
		"well_timed_shot": orNil(s.WellTimedShot),
	}
}

// A nil pointer in an any is not nil, which None is.
func orNil(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
