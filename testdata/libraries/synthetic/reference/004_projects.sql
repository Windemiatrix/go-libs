-- Case projects: projects (ZKIND 1508), which no test library holds.
-- Changes: of the user albums (ZKIND 2) outside the trash that have members, in ZUUID order, the first
-- becomes a project outside the trash and the second a project in the trash (ZTRASHEDSTATE 1).
-- Expects: the reference (golden-regen.yml applies this file too) has project_info for the photos of
-- the first and none for those of the second, and their album_info no longer lists either;
-- TestProjects and TestGolden find the two projects.

UPDATE ZGENERICALBUM SET ZKIND = 1508
WHERE Z_PK = (SELECT Z_PK FROM ZGENERICALBUM
              WHERE ZKIND = 2 AND ZTRASHEDSTATE = 0 AND ZUUID IS NOT NULL
                AND Z_PK IN (SELECT Z_33ALBUMS FROM Z_33ASSETS)
              ORDER BY ZUUID LIMIT 1);

UPDATE ZGENERICALBUM SET ZKIND = 1508, ZTRASHEDSTATE = 1
WHERE Z_PK = (SELECT Z_PK FROM ZGENERICALBUM
              WHERE ZKIND = 2 AND ZTRASHEDSTATE = 0 AND ZUUID IS NOT NULL
                AND Z_PK IN (SELECT Z_33ALBUMS FROM Z_33ASSETS)
              ORDER BY ZUUID LIMIT 1);
