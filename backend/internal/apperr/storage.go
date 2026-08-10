package apperr

// Storage-specific error constructors.

// BucketNameInvalid returns a 422 validation error for invalid bucket names.
func BucketNameInvalid(msg string) *Error {
	return Validation("bucket name is invalid", FieldIssue{Field: "name", Issue: msg})
}

// BucketAlreadyExists returns a 409 conflict when a bucket with this name exists.
func BucketAlreadyExists() *Error {
	return Conflict("bucket name already exists in this project")
}

// BucketNotActive returns a 409 conflict when an operation requires active status.
func BucketNotActive() *Error {
	return Conflict("bucket is not active")
}

// BucketNotEmpty returns a 409 conflict when a non-empty bucket cannot be deleted.
func BucketNotEmpty(count int64) *Error {
	return Conflict("BUCKET_NOT_EMPTY").WithDetails(FieldIssue{Field: "object_count", Issue: "non-zero"})
}

// BucketInactiveOperation returns a 403 error when modifying inactive buckets.
func BucketInactiveOperation() *Error {
	return Forbidden("cannot modify inactive bucket")
}

// BucketsLimitReached returns a 409 error when account bucket count exceeds quota.
func BucketsLimitReached(limit int) *Error {
	return Conflict("BUCKETS_LIMIT_REACHED").WithDetails(FieldIssue{Field: "bucket_count", Issue: "limit reached"})
}

// PermissionDenied returns a 403 permission denied error.
func PermissionDenied(msg string) *Error {
	return Forbidden(msg)
}
