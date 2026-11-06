package tvcompat

import (
	"strconv"
	"strings"
)

// sampleAspectRatioMismatch requires an explicitly signaled non-square ratio.
// Missing/unspecified values do not prove incompatibility: copying preserves the
// original H.264 and container display signaling without inventing a SAR value.
func sampleAspectRatioMismatch(value string) bool {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return false
	}
	numerator, numeratorErr := strconv.Atoi(parts[0])
	denominator, denominatorErr := strconv.Atoi(parts[1])
	if numeratorErr != nil || denominatorErr != nil || numerator <= 0 || denominator <= 0 {
		return false
	}
	return numerator != denominator
}
