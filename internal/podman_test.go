package internal

import (
	"testing"
)

func TestContainerExistsReturnsFalseWithoutPodman(t *testing.T) {
	exists := ContainerExists("stormdrain-nonexistent-container-xyz")
	if exists {
		t.Error("expected ContainerExists to return false for a container that should not exist")
	}
}
