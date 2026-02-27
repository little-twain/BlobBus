package blobbus

import "errors"

var (
	ErrLengthRequired = errors.New("length required")
	ErrTooLarge       = errors.New("payload too large")
	ErrNotFound       = errors.New("not found")
)
