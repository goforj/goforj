package forj

import "github.com/goforj/goforj/internal/logger"

// BuildCmd runs all build pipelines.
type BuildCmd struct {
	logger   *logger.AppLogger
	apiIndex *ApiIndexCmd
}

// NewBuildCmd creates a new build orchestrator command.
func NewBuildCmd(logger *logger.AppLogger, apiIndex *ApiIndexCmd) *BuildCmd {
	return &BuildCmd{
		logger:   logger,
		apiIndex: apiIndex,
	}
}

// Run executes all build:* commands in sequence.
func (c *BuildCmd) Run() error {
	steps := []struct {
		name string
		run  func() error
	}{
		{name: "build:api-index", run: c.apiIndex.Run},
	}

	for _, step := range steps {
		c.logger.Info().Any("step", step.name).Msg("Running build step")
		if err := step.run(); err != nil {
			c.logger.Error().Any("step", step.name).Err(err).Msg("Build step failed")
			return err
		}
	}

	c.logger.Info().Any("steps", len(steps)).Msg("Build completed")
	return nil
}
