-- Case asset-cloud-guid: PhotoInfo.CloudAssetGUID and IsCloudAsset.
-- The library has no iCloud: ZASSET.ZCLOUDASSETGUID is NULL for every asset.
-- Changes: the asset marked mig:asset-cloud-guid-1 gets the GUID TEST-GUID-0001; the assets marked
-- mig:asset-cloud-guid-dup-a and mig:asset-cloud-guid-dup-b both get TEST-GUID-0002.
-- Expects: -1 has CloudAssetGUID "TEST-GUID-0001" and IsCloudAsset true; both duplicates are returned with
-- the same CloudAssetGUID "TEST-GUID-0002" and the number of assets does not change (rejecting duplicates
-- is the job of the application, not of the library); every other asset has "" and false.

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:asset-cloud-guid-1'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZASSET SET ZCLOUDASSETGUID = 'TEST-GUID-0001'
WHERE Z_PK = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:asset-cloud-guid-1');

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:asset-cloud-guid-dup-a'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:asset-cloud-guid-dup-b'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZASSET SET ZCLOUDASSETGUID = 'TEST-GUID-0002'
WHERE Z_PK IN (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES
               WHERE ZTITLE IN ('mig:asset-cloud-guid-dup-a', 'mig:asset-cloud-guid-dup-b'));
