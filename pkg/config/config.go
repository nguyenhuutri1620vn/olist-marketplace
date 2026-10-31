// Package config reads service settings from environment variables.
package config

import (
	"fmt"
	"os"
	"strings"
)

// Require returns the values of the named environment variables. It fails with
// one error naming every variable that is unset or empty.
func Require(names ...string) (map[string]string, error) {
	values := make(map[string]string, len(names))
	var missing []string
	for _, n := range names {
		v := os.Getenv(n)
		if v == "" {
			missing = append(missing, n)
			continue
		}
		values[n] = v
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return values, nil
}
