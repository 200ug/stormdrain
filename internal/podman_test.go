package internal

import (
	"testing"
)

// container exists

func TestContainerExistsReturnsFalseWithoutPodman(t *testing.T) {
	exists := containerExists("stormdrain-nonexistent-container-xyz")
	if exists {
		t.Error("expected containerExists to return false for a container that should not exist")
	}
}
