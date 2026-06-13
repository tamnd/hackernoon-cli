package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) trendingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trending",
		Short: "Trending Hackernoon stories",
		RunE: func(cmd *cobra.Command, _ []string) error {
			n := a.effectiveLimit(20)
			a.progressf("fetching %d trending stories...", n)
			stories, err := a.client.Trending(cmd.Context(), n)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(stories, len(stories))
		},
	}
}
