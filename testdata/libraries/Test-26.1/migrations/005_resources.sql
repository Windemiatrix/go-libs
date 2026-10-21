-- Case resources: PhotoInfo.Resources and IsMissing (HasRAW is covered in Test-14.0.0: this library has
-- no RAW resource, data store subtype 17).
-- Changes: the photo
-- marked mig:resources-original (it has an original, subtype 1) gets every resource available locally and
-- remotely and ZDATALENGTH 1000001 on the original. The photo marked mig:resources-missing gets every
-- resource available only remotely. All resources of an asset change, not only the original: upstream
-- decides IsMissing by the last resource row it reads. ZINTERNALRESOURCE.ZFINGERPRINT is left as it is.
-- Expects: original: IsMissing
-- false and a resource {DataStoreSubtype 1, DataLength 1000001, LocalAvailability 1, RemoteAvailability 1};
-- missing: IsMissing true, Path "" and the subtype 1 resource with LocalAvailability 0. Path, PathLive and
-- PathRaw are empty for every asset: the test libraries have no originals.

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:resources-original'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
                AND a.ZKIND = 0
                AND EXISTS (SELECT 1 FROM ZINTERNALRESOURCE r WHERE r.ZASSET = a.Z_PK AND r.ZDATASTORESUBTYPE = 1)
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZINTERNALRESOURCE SET ZLOCALAVAILABILITY = 1, ZREMOTEAVAILABILITY = 1
WHERE ZASSET = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:resources-original');

UPDATE ZINTERNALRESOURCE SET ZDATALENGTH = 1000001
WHERE ZASSET = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:resources-original')
  AND ZDATASTORESUBTYPE = 1;

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:resources-missing'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
                AND a.ZKIND = 0
                AND EXISTS (SELECT 1 FROM ZINTERNALRESOURCE r WHERE r.ZASSET = a.Z_PK AND r.ZDATASTORESUBTYPE = 1)
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZINTERNALRESOURCE SET ZLOCALAVAILABILITY = 0, ZREMOTEAVAILABILITY = 1
WHERE ZASSET = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:resources-missing');
