package service

import "time"

// Clock returns the current time. Tests pass a fixed one.
type Clock func() time.Time
