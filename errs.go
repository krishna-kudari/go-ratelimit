package goratelimit

import (
	"fmt"
	"strings"
)

const docBase = "https://pkg.go.dev/github.com/krishna-kudari/ratelimit"

// validationErr returns an error with an actionable message and a doc link.
func validationErr(msg, suggestion string) error {
	return fmt.Errorf("goratelimit: %s. %s See %s", msg, suggestion, docBase)
}

// redisErr wraps a Redis backend error with a suggestion and optional Cluster hint.
func redisErr(err error, opts *Options) error {
	if err == nil {
		return nil
	}
	suggestion := "Check connection string and that Redis is reachable."
	if strings.Contains(strings.ToLower(err.Error()), "connection refused") {
		suggestion = "Is Redis running? Try: docker run -d -p 6379:6379 redis:alpine. See " + docBase + "#New"
	}
	if opts != nil && !opts.HashTag &&
		(strings.Contains(err.Error(), "CROSSSLOT") || strings.Contains(err.Error(), "MOVED")) {
		suggestion += " Using Redis Cluster? Enable WithHashTag(). See " + docBase + "#WithHashTag"
	}
	return fmt.Errorf("goratelimit: redis error: %w. %s", err, suggestion)
}
