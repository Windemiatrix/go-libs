-- Case skipped-asset: Info.SkippedAssets, an asset without its ZADDITIONALASSETATTRIBUTES row.
-- Upstream joins ZASSET with ZADDITIONALASSETATTRIBUTES and silently drops such an asset.
-- Changes: the last eligible asset in ZUUID order is marked in ZASSET with
-- ZFILENAME = 'mig-skipped-asset.jpg' (the title marker lives in the row that is deleted), then its
-- additional attributes row is deleted.
-- Expects: with S the number of assets that have no additional attributes row in the copy (S >= 1):
-- Info().SkippedAssets == S, len(Photos(Options{})) == COUNT(ZASSET) - S, and Photo(uuid) reports false
-- for each such asset, the marked one among them.

UPDATE ZASSET SET ZFILENAME = 'mig-skipped-asset.jpg'
WHERE Z_PK = (SELECT a.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID DESC LIMIT 1);

DELETE FROM ZADDITIONALASSETATTRIBUTES
WHERE ZASSET = (SELECT Z_PK FROM ZASSET WHERE ZFILENAME = 'mig-skipped-asset.jpg');
