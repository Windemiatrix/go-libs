-- Case asset-cloud-guid-absent: CloudAssetGUID NULL and empty in an iCloud library.
-- Every asset of this library has a cloud GUID.
-- Changes: the asset marked mig:asset-cloud-guid-null gets ZCLOUDASSETGUID NULL; the asset marked
-- mig:asset-cloud-guid-empty gets an empty string.
-- Expects: both have CloudAssetGUID "" and IsCloudAsset false. An empty string means no GUID; this is an
-- intended difference from upstream, where an empty GUID still makes the asset a cloud asset.

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:asset-cloud-guid-null'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZASSET SET ZCLOUDASSETGUID = NULL
WHERE Z_PK = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:asset-cloud-guid-null');

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:asset-cloud-guid-empty'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZASSET SET ZCLOUDASSETGUID = ''
WHERE Z_PK = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:asset-cloud-guid-empty');
