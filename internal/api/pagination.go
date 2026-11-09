package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

type pageCursor struct {
	Time time.Time `json:"t"`
	ID   string    `json:"i"`
}

func pageParams(r *http.Request) (int, *pageCursor, error) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return 0, nil, errors.New("limit must be 1 to 100")
		}
		limit = n
	}
	raw := r.URL.Query().Get("cursor")
	if raw == "" {
		return limit, nil, nil
	}
	if len(raw) > 512 {
		return 0, nil, errors.New("invalid cursor")
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return 0, nil, errors.New("invalid cursor")
	}
	var c pageCursor
	if json.Unmarshal(b, &c) != nil || c.Time.IsZero() || len(c.ID) != 36 {
		return 0, nil, errors.New("invalid cursor")
	}
	return limit, &c, nil
}

func encodeCursor(t time.Time, id string) string {
	b, _ := json.Marshal(pageCursor{Time: t, ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}
