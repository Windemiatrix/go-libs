package scoreinfo

// AsDict is asdict of ScoreInfo (scoreinfo.py:42-44): each score a float64, 0
// where upstream has None, a NULL column, and for behavioral and interaction
// from Photos 11 on (_photosdb_process_scoreinfo.py:62-76). The map is new on
// every call.
func (s ScoreInfo) AsDict() map[string]any {
	return map[string]any{
		"overall": s.Overall, "curation": s.Curation, "promotion": s.Promotion, "highlight_visibility": s.HighlightVisibility,
		"behavioral": s.Behavioral, "failure": s.Failure, "harmonious_color": s.HarmoniousColor, "immersiveness": s.Immersiveness,
		"interaction": s.Interaction, "interesting_subject": s.InterestingSubject, "intrusive_object_presence": s.IntrusiveObjectPresence,
		"lively_color": s.LivelyColor, "low_light": s.LowLight, "noise": s.Noise, "pleasant_camera_tilt": s.PleasantCameraTilt,
		"pleasant_composition": s.PleasantComposition, "pleasant_lighting": s.PleasantLighting, "pleasant_pattern": s.PleasantPattern,
		"pleasant_perspective": s.PleasantPerspective, "pleasant_post_processing": s.PleasantPostProcessing,
		"pleasant_reflection": s.PleasantReflection, "pleasant_symmetry": s.PleasantSymmetry,
		"sharply_focused_subject": s.SharplyFocusedSubject, "tastefully_blurred": s.TastefullyBlurred,
		"well_chosen_subject": s.WellChosenSubject, "well_framed_subject": s.WellFramedSubject, "well_timed_shot": s.WellTimedShot,
	}
}
