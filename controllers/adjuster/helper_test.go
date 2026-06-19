package adjuster

import (
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func TestIsNotFound(t *testing.T) {
	t.Log("isNotFound(nil)=", apierrors.IsNotFound(nil))
}
