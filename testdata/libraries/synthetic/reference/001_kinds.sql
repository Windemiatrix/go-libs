-- Case kinds: SlowMo, ScreenRecording and BurstDefaultPick, which no test library holds, in the reference.
-- Changes: the first movie in ZUUID order without a subtype gets ZKINDSUBTYPE 101 (slow motion), the next
-- one 103 (screen recording); the first selected or key burst frame without the default bit gets it (4).
-- Expects: the reference (golden-regen.yml applies this file too) has slow_mo, screen_recording and
-- burst_default_pick true for one object each, and TestGolden finds no difference.

UPDATE ZASSET SET ZKINDSUBTYPE = 101
WHERE Z_PK = (SELECT Z_PK FROM ZASSET
              WHERE ZTRASHEDSTATE = 0 AND ZKIND = 1 AND COALESCE(ZKINDSUBTYPE, 0) = 0
              ORDER BY ZUUID LIMIT 1);

UPDATE ZASSET SET ZKINDSUBTYPE = 103
WHERE Z_PK = (SELECT Z_PK FROM ZASSET
              WHERE ZTRASHEDSTATE = 0 AND ZKIND = 1 AND COALESCE(ZKINDSUBTYPE, 0) = 0
              ORDER BY ZUUID LIMIT 1);

UPDATE ZASSET SET ZAVALANCHEPICKTYPE = ZAVALANCHEPICKTYPE | 4
WHERE Z_PK = (SELECT Z_PK FROM ZASSET
              WHERE ZTRASHEDSTATE = 0 AND ZAVALANCHEUUID IS NOT NULL
                AND ZAVALANCHEPICKTYPE & 24 != 0 AND ZAVALANCHEPICKTYPE & 4 = 0
              ORDER BY ZUUID LIMIT 1);
