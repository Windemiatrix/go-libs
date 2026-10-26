-- Case burst-frames: BurstPickType, the selection of burst frames by Query and BurstAlbumInfoOf.
-- Album membership table of schema photos8: Z_28ASSETS (album Z_28ALBUMS, asset Z_3ASSETS).
-- Changes: six photos outside any burst become one burst with ZAVALANCHEUUID TEST-BURST-0001 and the pick
-- types mig:burst-key 16, mig:burst-selected 8, mig:burst-none 0, mig:burst-unselected 2,
-- mig:burst-default 4, mig:burst-null NULL. The key frame is chosen first, among the photos that belong to
-- a user album; the other five are removed from every album.
-- Expects: all six have Burst true and BurstUUID "TEST-BURST-0001", and BurstAlbumInfoOf equal to
-- AlbumInfoOf of the key frame (not empty); AlbumUUIDs of the other five is empty. BurstKey for 16,
-- BurstSelected for 8. Query(query.Options{}) has key, selected, none and null, and not unselected (2)
-- and default (4); Photos() has all six.

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:burst-key'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
                AND a.ZKIND = 0
                AND EXISTS (SELECT 1 FROM Z_28ASSETS m JOIN ZGENERICALBUM g ON g.Z_PK = m.Z_28ALBUMS
                            WHERE m.Z_3ASSETS = a.Z_PK AND g.ZKIND = 2 AND g.ZTRASHEDSTATE = 0)
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:burst-selected'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
                AND a.ZKIND = 0
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:burst-none'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
                AND a.ZKIND = 0
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:burst-unselected'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
                AND a.ZKIND = 0
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:burst-default'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
                AND a.ZKIND = 0
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:burst-null'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
                AND a.ZKIND = 0
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZASSET SET ZAVALANCHEUUID = 'TEST-BURST-0001'
WHERE Z_PK IN (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES
               WHERE ZTITLE IN ('mig:burst-key', 'mig:burst-selected', 'mig:burst-none',
                                'mig:burst-unselected', 'mig:burst-default', 'mig:burst-null'));

UPDATE ZASSET SET ZAVALANCHEPICKTYPE = 16
WHERE Z_PK = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:burst-key');

UPDATE ZASSET SET ZAVALANCHEPICKTYPE = 8
WHERE Z_PK = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:burst-selected');

UPDATE ZASSET SET ZAVALANCHEPICKTYPE = 0
WHERE Z_PK = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:burst-none');

UPDATE ZASSET SET ZAVALANCHEPICKTYPE = 2
WHERE Z_PK = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:burst-unselected');

UPDATE ZASSET SET ZAVALANCHEPICKTYPE = 4
WHERE Z_PK = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:burst-default');

UPDATE ZASSET SET ZAVALANCHEPICKTYPE = NULL
WHERE Z_PK = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:burst-null');

-- The five frames other than the key frame may already belong to no album.
-- rows: any
DELETE FROM Z_28ASSETS
WHERE Z_3ASSETS IN (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES
                    WHERE ZTITLE IN ('mig:burst-selected', 'mig:burst-none', 'mig:burst-unselected',
                                     'mig:burst-default', 'mig:burst-null'));
