package app

import "context"

// Reserve exposes the app's reservation step to the external tests.
func (a *App) Reserve(ctx context.Context, userID, amount int64) (string, error) {
	return a.reserve(ctx, userID, amount)
}
