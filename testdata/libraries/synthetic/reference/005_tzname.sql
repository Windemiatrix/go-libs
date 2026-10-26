-- Case tzname: a time zone name whose zone has another offset than ZTIMEZONEOFFSET, which no test
-- library holds.
-- Changes: of the plain photos (outside the trash, not in a burst, without a subtype, with attributes)
-- in ZUUID order after 002_columns.sql, the 15th, which no earlier file changes, gets ZTIMEZONENAME
-- 'Europe/Paris' and ZTIMEZONEOFFSET 10800, never an offset of Paris, so its date stays put.
-- Expects: the reference (golden-regen.yml applies this file too) has tzoffset 10800, tzname
-- Europe/Paris and date with the offset of Paris at that moment (+01:00 or +02:00); TestGolden and
-- TestDateOffsetGolden find no difference.

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTIMEZONENAME = 'Europe/Paris', ZTIMEZONEOFFSET = 10800
WHERE ZASSET = (SELECT Z_PK FROM ZASSET
                WHERE ZTRASHEDSTATE = 0 AND ZKIND = 0 AND COALESCE(ZKINDSUBTYPE, 0) = 0 AND ZAVALANCHEUUID IS NULL
                  AND Z_PK IN (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES)
                ORDER BY ZUUID LIMIT 1 OFFSET 14);
