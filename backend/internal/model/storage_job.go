package model

// Storage job kinds for the durable queue protocol.
const (
	JobProvisionStorageBucket = "provision_storage_bucket"
	JobDeleteStorageBucket    = "delete_storage_bucket"
	JobReconcileStorageBucket = "reconcile_storage_bucket"
)

// Max retry attempts for storage bucket jobs.
const (
	MaxProvisionAttempts = 5
	MaxDeleteAttempts    = 5
	MaxReconcileAttempts = 3
)
