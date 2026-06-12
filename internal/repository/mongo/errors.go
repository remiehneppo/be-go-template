package mongo

import "github.com/remihneppo/be-go-template/internal/platform/database"

func mapWriteError(err error) error {
	if database.IsDuplicateKeyError(err) {
		return database.ErrConflict
	}
	return err
}
