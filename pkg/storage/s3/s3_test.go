//go:build integration || s3
// +build integration s3

package s3

import (
	"testing"

	"github.com/nimbolus/terraform-backend/pkg/storage/util"
)

func TestStorage(t *testing.T) {
	s, err := NewS3Storage("localhost:9000", "terraform-backend", "root", "password", false)
	if err != nil {
		t.Error(err)
	}

	util.StorageTest(t, s)
}
