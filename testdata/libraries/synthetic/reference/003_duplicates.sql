-- Case duplicates: two plain photos with one duplicate signature, which no test library holds.
-- Changes: of the plain photos (outside the trash, not in a burst, without a subtype, with attributes)
-- in ZUUID order after 002_columns.sql, the 14th gets the original file size, date, height, width, UTI
-- and adjustments state of the 13th (photosdb.py:3411-3420).
-- Expects: the reference (golden-regen.yml applies this file too) has the copied values, and
-- TestQueryDuplicates finds this pair as a group.

UPDATE ZADDITIONALASSETATTRIBUTES
SET ZORIGINALFILESIZE = (SELECT x.ZORIGINALFILESIZE FROM ZADDITIONALASSETATTRIBUTES x
                         WHERE x.ZASSET = (SELECT Z_PK FROM ZASSET
                                           WHERE ZTRASHEDSTATE = 0 AND ZKIND = 0 AND COALESCE(ZKINDSUBTYPE, 0) = 0 AND ZAVALANCHEUUID IS NULL
                                             AND Z_PK IN (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES)
                                           ORDER BY ZUUID LIMIT 1 OFFSET 12))
WHERE ZASSET = (SELECT Z_PK FROM ZASSET
                WHERE ZTRASHEDSTATE = 0 AND ZKIND = 0 AND COALESCE(ZKINDSUBTYPE, 0) = 0 AND ZAVALANCHEUUID IS NULL
                  AND Z_PK IN (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES)
                ORDER BY ZUUID LIMIT 1 OFFSET 13);

UPDATE ZASSET
SET (ZDATECREATED, ZHEIGHT, ZWIDTH, ZUNIFORMTYPEIDENTIFIER, ZADJUSTMENTSSTATE) =
    (SELECT s.ZDATECREATED, s.ZHEIGHT, s.ZWIDTH, s.ZUNIFORMTYPEIDENTIFIER, s.ZADJUSTMENTSSTATE FROM ZASSET s
     WHERE s.Z_PK = (SELECT Z_PK FROM ZASSET
                     WHERE ZTRASHEDSTATE = 0 AND ZKIND = 0 AND COALESCE(ZKINDSUBTYPE, 0) = 0 AND ZAVALANCHEUUID IS NULL
                       AND Z_PK IN (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES)
                     ORDER BY ZUUID LIMIT 1 OFFSET 12))
WHERE Z_PK = (SELECT Z_PK FROM ZASSET
              WHERE ZTRASHEDSTATE = 0 AND ZKIND = 0 AND COALESCE(ZKINDSUBTYPE, 0) = 0 AND ZAVALANCHEUUID IS NULL
                AND Z_PK IN (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES)
              ORDER BY ZUUID LIMIT 1 OFFSET 13);
