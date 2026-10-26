-- Case fingerprint-schema: PhotoInfo.Fingerprint read from the fingerprint column of the schema.
-- Schema photos8 keeps the fingerprint of the original in ZADDITIONALASSETATTRIBUTES.ZMASTERFINGERPRINT.
-- Changes: the asset marked mig:fingerprint-value gets the fingerprint TEST-FP-0001; the asset marked
-- mig:fingerprint-null gets NULL. ZINTERNALRESOURCE.ZFINGERPRINT is left as it is.
-- Expects: value: Fingerprint "TEST-FP-0001", read from ZMASTERFINGERPRINT;
-- null: Fingerprint "". IsMissing of these assets is not asserted.

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:fingerprint-value', ZMASTERFINGERPRINT = 'TEST-FP-0001'
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID LIMIT 1);

UPDATE ZADDITIONALASSETATTRIBUTES SET ZTITLE = 'mig:fingerprint-null', ZMASTERFINGERPRINT = NULL
WHERE Z_PK = (SELECT x.Z_PK FROM ZADDITIONALASSETATTRIBUTES x JOIN ZASSET a ON a.Z_PK = x.ZASSET
              WHERE (x.ZTITLE IS NULL OR x.ZTITLE NOT LIKE 'mig:%')
                AND a.ZTRASHEDSTATE = 0 AND a.ZAVALANCHEUUID IS NULL
              ORDER BY a.ZUUID LIMIT 1);
