-- Case trash: assets in the trash are not filtered out implicitly.
-- Changes: the asset marked mig:trash is moved to the trash on 2023-01-01T00:00:00Z
-- (ZTRASHEDSTATE 1, ZTRASHEDDATE 694224000 Core Data seconds).
-- Expects: InTrash true and DateTrashed 2023-01-01T00:00:00Z; the asset is in Photos(Options{}) and in
-- Photos(Options{TrashedOnly: true}) and not in Photos(OSXPhotosDefaults()).

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:trash'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZASSET SET ZTRASHEDSTATE = 1, ZTRASHEDDATE = 694224000
WHERE Z_PK = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:trash');
