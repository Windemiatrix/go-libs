-- Case shared-album: a shared album of Photos 11 and later, a CollectionShare row of ZSHARE with photos,
-- which no test library holds (photosdb.py:2787-2916).
-- Changes: the ZSHARE row with a ZUUID and the highest Z_PK becomes a CollectionShare (the entity of
-- Z_PRIMARYKEY: without it the statement changes no row) outside the trash, sorted by hand, titled
-- Shared album (with a NULL title upstream would write null into albums); of the plain
-- photos (outside the trash, not in a burst, without a subtype, with attributes) in ZUUID order after
-- 002_columns.sql, the 16th, 17th and 18th, which no earlier file changes, join it with publish dates
-- in the reverse order. The library has no ZSHAREPARTICIPANT row, so the album has no owner.
-- Expects: the reference (golden-regen.yml applies this file too) lists the album in album_info and its
-- title in albums of the three photos, without an owner; TestGolden finds no difference,
-- TestSharedAlbums and TestOpenCollectionShare find the album.

UPDATE ZSHARE SET Z_ENT = (SELECT Z_ENT FROM Z_PRIMARYKEY WHERE Z_NAME = 'CollectionShare'),
                  ZTITLE = 'Shared album', ZTRASHEDSTATE = 0, ZCUSTOMSORTKEY = 0
WHERE Z_PK = (SELECT MAX(Z_PK) FROM ZSHARE WHERE ZUUID IS NOT NULL)
  AND EXISTS (SELECT 1 FROM Z_PRIMARYKEY WHERE Z_NAME = 'CollectionShare');

UPDATE ZASSET SET ZCOLLECTIONSHARE = (SELECT MAX(Z_PK) FROM ZSHARE
                                      WHERE Z_ENT = (SELECT Z_ENT FROM Z_PRIMARYKEY WHERE Z_NAME = 'CollectionShare')),
                  ZCLOUDBATCHPUBLISHDATE = 694224300
WHERE Z_PK = (SELECT Z_PK FROM ZASSET
              WHERE ZTRASHEDSTATE = 0 AND ZKIND = 0 AND COALESCE(ZKINDSUBTYPE, 0) = 0 AND ZAVALANCHEUUID IS NULL
                AND Z_PK IN (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES)
              ORDER BY ZUUID LIMIT 1 OFFSET 15);

UPDATE ZASSET SET ZCOLLECTIONSHARE = (SELECT MAX(Z_PK) FROM ZSHARE
                                      WHERE Z_ENT = (SELECT Z_ENT FROM Z_PRIMARYKEY WHERE Z_NAME = 'CollectionShare')),
                  ZCLOUDBATCHPUBLISHDATE = 694224200
WHERE Z_PK = (SELECT Z_PK FROM ZASSET
              WHERE ZTRASHEDSTATE = 0 AND ZKIND = 0 AND COALESCE(ZKINDSUBTYPE, 0) = 0 AND ZAVALANCHEUUID IS NULL
                AND Z_PK IN (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES)
              ORDER BY ZUUID LIMIT 1 OFFSET 16);

UPDATE ZASSET SET ZCOLLECTIONSHARE = (SELECT MAX(Z_PK) FROM ZSHARE
                                      WHERE Z_ENT = (SELECT Z_ENT FROM Z_PRIMARYKEY WHERE Z_NAME = 'CollectionShare')),
                  ZCLOUDBATCHPUBLISHDATE = 694224100
WHERE Z_PK = (SELECT Z_PK FROM ZASSET
              WHERE ZTRASHEDSTATE = 0 AND ZKIND = 0 AND COALESCE(ZKINDSUBTYPE, 0) = 0 AND ZAVALANCHEUUID IS NULL
                AND Z_PK IN (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES)
              ORDER BY ZUUID LIMIT 1 OFFSET 17);
