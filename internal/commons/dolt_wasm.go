//go:build js

package commons

import (
	"fmt"
	"io"
)

// FetchRemote is unavailable in js builds because it shells out to dolt.
func FetchRemote(dbDir, remote string) error {
	return fmt.Errorf("dolt fetch unavailable in js build: %s %s", dbDir, remote)
}

// PushBranchToRemoteForce is unavailable in js builds because it shells out to dolt.
func PushBranchToRemoteForce(dbDir, remote, branch string, force bool, stdout io.Writer) error {
	return fmt.Errorf("dolt push unavailable in js build: %s %s %s", dbDir, remote, branch)
}
