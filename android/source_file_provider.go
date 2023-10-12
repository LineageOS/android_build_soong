package android

import (
	"github.com/google/blueprint"
)

type SrcsFileProviderData struct {
	SrcPaths Paths
}

var SrcsFileProviderKey = blueprint.NewProvider(SrcsFileProviderData{})
