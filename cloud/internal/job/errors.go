package job

import "errors"

var (
	ErrInvalidJobID            = errors.New("invalid job_id")
	ErrInvalidStatus           = errors.New("invalid status")
	ErrInvalidInput            = errors.New("invalid input (bucket and key required)")
	ErrInvalidOutput           = errors.New("invalid output (bucket required, key or prefix required)")
	ErrInvalidAttemptID        = errors.New("invalid attempt_id (must be >= 1)")
	ErrInvalidJobType          = errors.New("invalid job type")
	ErrInvalidForwardURL       = errors.New("invalid forward_url")
	ErrInvalidInputForwardMode = errors.New("invalid input_forward_mode")
	ErrInvalidTransition       = errors.New("invalid status transition")
	ErrJobNotFound             = errors.New("job not found")
	ErrJobAlreadyExists        = errors.New("job already exists")
)
