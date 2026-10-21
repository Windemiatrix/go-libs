-- Case album-cloud-guid (NULL in an iCloud library): AlbumInfo.CloudGUID.
-- Changes: the first user album (ZKIND 2) not in the trash is renamed mig:album-cloud-guid-null and gets
-- ZCLOUDGUID NULL.
-- Expects: the album titled mig:album-cloud-guid-null has CloudGUID "".

UPDATE ZGENERICALBUM SET ZTITLE = 'mig:album-cloud-guid-null', ZCLOUDGUID = NULL
WHERE Z_PK = (SELECT Z_PK FROM ZGENERICALBUM
              WHERE ZKIND = 2 AND ZTRASHEDSTATE = 0 AND (ZTITLE IS NULL OR ZTITLE NOT LIKE 'mig:%')
              ORDER BY ZUUID LIMIT 1);
