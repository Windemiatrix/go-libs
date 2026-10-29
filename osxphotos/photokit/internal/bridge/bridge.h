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
