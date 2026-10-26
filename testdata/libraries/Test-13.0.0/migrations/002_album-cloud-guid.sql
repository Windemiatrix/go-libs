-- Case album-cloud-guid: AlbumInfo.CloudGUID and FolderInfo.CloudGUID.
-- The library has no iCloud.
-- Changes: the first user album (ZKIND 2) not in the trash is renamed mig:album-cloud-guid and gets
-- the GUID TEST-ALBUM-GUID-0001; the first folder (ZKIND 4000, not the root folder 3999) is renamed
-- mig:folder-cloud-guid and gets TEST-FOLDER-GUID-0001.
-- Expects: AlbumInfo() has the album titled mig:album-cloud-guid with CloudGUID "TEST-ALBUM-GUID-0001";
-- Folders() has the folder titled mig:folder-cloud-guid with CloudGUID "TEST-FOLDER-GUID-0001";
-- other albums have CloudGUID "".

UPDATE ZGENERICALBUM SET ZTITLE = 'mig:album-cloud-guid', ZCLOUDGUID = 'TEST-ALBUM-GUID-0001'
WHERE Z_PK = (SELECT Z_PK FROM ZGENERICALBUM
              WHERE ZKIND = 2 AND ZTRASHEDSTATE = 0 AND (ZTITLE IS NULL OR ZTITLE NOT LIKE 'mig:%')
              ORDER BY ZUUID LIMIT 1);

UPDATE ZGENERICALBUM SET ZTITLE = 'mig:folder-cloud-guid', ZCLOUDGUID = 'TEST-FOLDER-GUID-0001'
WHERE Z_PK = (SELECT Z_PK FROM ZGENERICALBUM
              WHERE ZKIND = 4000 AND ZTRASHEDSTATE = 0 AND (ZTITLE IS NULL OR ZTITLE NOT LIKE 'mig:%')
              ORDER BY ZUUID LIMIT 1);
