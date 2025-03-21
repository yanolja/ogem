package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMustWithoutOutput(t *testing.T) {
	t.Run("should not panic when error is nil", func(t *testing.T) {
		MustWithoutOutput(nil)
	})
	t.Run("should panic when error is not nil", func(t *testing.T) {
		assert.Panics(t, func() {
			MustWithoutOutput(fmt.Errorf("test error"))
		})
	})
}
