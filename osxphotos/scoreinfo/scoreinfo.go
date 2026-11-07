package scoreinfo

// ScoreInfo is the computed scores of a photo, the ScoreInfo of osxphotos
// (scoreinfo.py:10-40): the ZCOMPUTEDASSETATTRIBUTES row of the asset and four
// scores of its ZASSET row (photosdb/_photosdb_process_scoreinfo.py:53-98).
// Every score is 0 for a photo without a row, as upstream gives 0.0 there
// (photoinfo.py:1507-1537), and for a NULL column, where upstream keeps None;
// Behavioral and Interaction are 0 from Photos 11 on, whose database has no
// such columns (upstream selects null in their place,
// _photosdb_process_scoreinfo.py:62-76). The order of the fields is that of
// scoreinfo.py:14-40.
type ScoreInfo struct {
	Overall, Curation, Promotion, HighlightVisibility float64 // ZASSET.ZOVERALLAESTHETICSCORE (overall_aesthetic), ZCURATIONSCORE, ZPROMOTIONSCORE, ZHIGHLIGHTVISIBILITYSCORE

	Behavioral, Failure, HarmoniousColor, Immersiveness, Interaction, InterestingSubject, IntrusiveObjectPresence float64

	LivelyColor, LowLight, Noise, PleasantCameraTilt, PleasantComposition, PleasantLighting, PleasantPattern float64

	PleasantPerspective, PleasantPostProcessing, PleasantReflection, PleasantSymmetry, SharplyFocusedSubject float64

	TastefullyBlurred, WellChosenSubject, WellFramedSubject, WellTimedShot float64
}
