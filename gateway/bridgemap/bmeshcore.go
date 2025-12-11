//go:build !nomeshcore
// +build !nomeshcore

package bridgemap

import (
	bmeshcore "github.com/matterbridge-org/matterbridge/bridge/meshcore"
)

func init() {
	FullMap["meshcore"] = bmeshcore.New
}
