package storage

import (
	"encoding/json"

	"github.com/cloud-print/server/internal/domain"
)

func jsonMarshalParams(p domain.PrintParams) ([]byte, error) {
	return json.Marshal(p)
}

func unmarshalParams(s string) domain.PrintParams {
	if s == "" {
		return domain.PrintParams{}
	}
	var p domain.PrintParams
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return domain.PrintParams{}
	}
	return p
}