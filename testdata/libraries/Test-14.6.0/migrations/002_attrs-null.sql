-- Case attrs-null: NULL in the time zone offset, the original file name and the creation date.
-- Changes: the asset marked mig:tzoffset-null gets ZADDITIONALASSETATTRIBUTES.ZTIMEZONEOFFSET NULL;
-- mig:original-filename-null gets ZORIGINALFILENAME NULL and mig:original-filename-empty an empty string;
-- mig:date-created-null gets ZASSET.ZDATECREATED NULL.
-- Expects: tzoffset-null: TZOffset nil and Date at offset 0, as upstream; original-filename-null and
-- original-filename-empty: OriginalFilename equal to Filename; date-created-null: Date 1970-01-01T00:00:00Z,
-- the default of upstream.

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:tzoffset-null', ZTIMEZONEOFFSET = NULL
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:original-filename-null', ZORIGINALFILENAME = NULL
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:original-filename-empty', ZORIGINALFILENAME = ''
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:date-created-null'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZASSET SET ZDATECREATED = NULL
WHERE Z_PK = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:date-created-null');
