package scoreinfo

// ScoreInfo is the computed scores of a photo, the ScoreInfo of osxphotos
// (scoreinfo.py:10-40): the ZCOMPUTEDASSETATTRIBUTES row of the asset and four
// scores of its ZASSET row (photosdb/_photosdb_process_scoreinfo.py:53-98).
// A score is nil where upstream has None: a NULL column, and Behavioral and
// Interaction from Photos 11 on, whose database has no such columns (upstream
// selects null in their place, _photosdb_process_scoreinfo.py:62-76). A photo
// without a row has every score 0.0, not nil, as upstream
// (photoinfo.py:1507-1537); the zero value has every score nil. The order of
// the fields is that of scoreinfo.py:14-40.
type ScoreInfo struct {
	Overall, Curation, Promotion, HighlightVisibility *float64 // ZASSET.ZOVERALLAESTHETICSCORE (overall_aesthetic), ZCURATIONSCORE, ZPROMOTIONSCORE, ZHIGHLIGHTVISIBILITYSCORE

	Behavioral, Failure, HarmoniousColor, Immersiveness, Interaction, InterestingSubject, IntrusiveObjectPresence *float64

	LivelyColor, LowLight, Noise, PleasantCameraTilt, PleasantComposition, PleasantLighting, PleasantPattern *float64

	PleasantPerspective, PleasantPostProcessing, PleasantReflection, PleasantSymmetry, SharplyFocusedSubject *float64

	TastefullyBlurred, WellChosenSubject, WellFramedSubject, WellTimedShot *float64
}
