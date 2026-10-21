-- Case cloud-master: PhotoInfo.InCloud (*bool) and CloudMasterGUID.
-- In this library every asset with a master (ZASSET.ZMASTER -> ZCLOUDMASTER) is in iCloud.
-- Changes: the master of the asset marked mig:cloud-master-local gets ZCLOUDLOCALSTATE 1 (not in iCloud)
-- and ZCLOUDMASTERGUID TEST-MASTER-GUID-0001; the asset marked mig:cloud-master-none loses its master
-- (ZMASTER NULL).
-- Expects: local: InCloud not nil and false, CloudMasterGUID "TEST-MASTER-GUID-0001"; none: InCloud nil,
-- CloudMasterGUID "".

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:cloud-master-local'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
                AND a.ZMASTER IS NOT NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZCLOUDMASTER SET ZCLOUDLOCALSTATE = 1, ZCLOUDMASTERGUID = 'TEST-MASTER-GUID-0001'
WHERE Z_PK = (SELECT a.ZMASTER FROM ZASSET a JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK
              WHERE x.ZTITLE = 'mig:cloud-master-local');

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:cloud-master-none'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
                AND a.ZMASTER IS NOT NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZASSET SET ZMASTER = NULL
WHERE Z_PK = (SELECT ZASSET FROM ZADDITIONALASSETATTRIBUTES WHERE ZTITLE = 'mig:cloud-master-none');
