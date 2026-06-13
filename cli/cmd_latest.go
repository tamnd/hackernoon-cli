package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) latestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "latest",
		Short: "Latest Hackernoon stories",
		RunE: func(cmd *cobra.Command, _ []string) error {
			n := a.effectiveLimit(20)
			a.progressf("fetching %d latest stories...", n)
			stories, err := a.client.Latest(cmd.Context(), n)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(stories, len(stories))
		},
	}
}
