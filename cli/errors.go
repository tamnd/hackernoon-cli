package cli

import (
	"errors"

	"github.com/tamnd/hackernoon-cli/hackernoon"
)

func isNotFound(err error) bool {
	return errors.Is(err, hackernoon.ErrNotFound)
}

func mapFetchErr(err error) error {
	if err == nil {
		return nil
	}
	if isNotFound(err) {
		return codeError(exitNoData, err)
	}
	return codeError(exitError, err)
}
