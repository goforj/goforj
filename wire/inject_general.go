package wire

import (
	"github.com/goforj/goforj/internal/goforj"
	"github.com/google/wire"
)

var generalSet = wire.NewSet(
	goforj.NewProjectRenderer,
)
