package models

import "github.com/google/uuid"

func ensureUUID(id *uuid.UUID) {
	if id == nil {
		return
	}
	if *id == uuid.Nil {
		*id = uuid.New()
	}
}
