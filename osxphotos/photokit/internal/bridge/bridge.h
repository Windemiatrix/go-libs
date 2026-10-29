#include <stdbool.h>
#include <stdint.h>

enum { PK_OK, PK_EXCEPTION, PK_TIMEOUT };

// message is NULL unless code is PK_EXCEPTION; the caller frees it.
typedef struct {
	int code;
	char *message;
} pk_result;

pk_result pk_authorization_status(int *status);
// A timeout_ms of 0 does not wait.
pk_result pk_request_authorization(int64_t timeout_ms, int *status);

// A string is NULL when PhotoKit gives none; a date or the coordinates hold only
// with their has_ field. Dates are seconds since 1970, the duration seconds.
typedef struct {
	char *local_identifier, *burst_identifier;
	int media_type, source_type, pixel_width, pixel_height;
	uint64_t media_subtypes;
	bool has_creation_date, has_modification_date, has_location, favorite, hidden;
	double creation_date, modification_date, latitude, longitude, duration;
} pk_asset;

// Both find hidden assets too. *out holds *count assets in the order of
// PhotoKit, or is NULL with *count 0 on an error or when none is found;
// pk_free_assets frees it in every case.
pk_result pk_fetch_assets(const char *const *ids, int64_t n, pk_asset **out, int *count);
pk_result pk_fetch_burst(const char *burst_id, bool include_all, pk_asset **out, int *count);
void pk_free_assets(pk_asset *assets, int count);
