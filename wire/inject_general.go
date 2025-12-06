package wire

import (
	"github.com/goforj/goforj/internal/forj"
	"github.com/google/wire"
)

var generalSet = wire.NewSet(
	forj.NewProjectRenderer,
)
