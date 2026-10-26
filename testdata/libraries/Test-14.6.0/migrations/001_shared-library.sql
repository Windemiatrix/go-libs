-- Case shared-library: PhotoInfo.SharedLibrary and LibraryScopeShareState, DB.ShareInfo.
-- The library is not a shared library.
-- Changes: the asset marked mig:shared-library-member gets ZACTIVELIBRARYSCOPEPARTICIPATIONSTATE 1,
-- ZLIBRARYSCOPESHARESTATE 2 and ZLIBRARYSCOPE 999999, a key of no ZSHARE row; the asset marked
-- mig:shared-library-null gets NULL in all three columns.
-- Expects: member: SharedLibrary true, LibraryScopeShareState 2, ShareInfo nil; null: false, 0 and nil.
-- Info().SupportsSharedLibrary is true without any migration.

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:shared-library-member'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZASSET SET ZACTIVELIBRARYSCOPEPARTICIPATIONSTATE = 1, ZLIBRARYSCOPESHARESTATE = 2, ZLIBRARYSCOPE = 999999
WHERE Z_PK = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:shared-library-member');

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:shared-library-null'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZASSET SET ZACTIVELIBRARYSCOPEPARTICIPATIONSTATE = NULL, ZLIBRARYSCOPESHARESTATE = NULL, ZLIBRARYSCOPE = NULL
WHERE Z_PK = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:shared-library-null');
