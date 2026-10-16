package main

import (
	"encoding/json"
	"net/http"
)

type adminBeadWork struct {
	beadWork
	AccountID string `json:"account_id"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
}

func (a *app) adminBeadList(w http.ResponseWriter, r *http.Request) {
	page, ok := pagination(w, r)
	if !ok {
		return
	}
	q, ok := searchQuery(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "finished" && status != "ongoing" {
		fail(w, 400, "作品状态不正确")
		return
	}
	owner := r.URL.Query().Get("account_id")
	const from = ` FROM bead_works b JOIN accounts a ON a.id=b.account_id WHERE ($1='' OR strpos(lower(a.username),lower($1))>0 OR strpos(lower(a.nickname),lower($1))>0 OR strpos(lower(b.data->>'name'),lower($1))>0) AND ($2='' OR b.account_id=$2) AND ($3='all' OR ($3='finished' AND COALESCE((b.data->>'finished')::boolean,false)) OR ($3='ongoing' AND NOT COALESCE((b.data->>'finished')::boolean,false)))`
	var total int
	if err := a.db.QueryRow(r.Context(), "SELECT count(*)"+from, q, owner, status).Scan(&total); err != nil {
		a.internal(w, err)
		return
	}
	// Keep each gallery page bounded; only placed beads are needed for previews.
	rows, err := a.db.Query(r.Context(), `SELECT b.id,b.revision,b.data-'target',b.updated_at,a.id,a.username,COALESCE(NULLIF(a.nickname,''),a.username)`+from+` ORDER BY b.updated_at DESC,b.account_id,b.id LIMIT 12 OFFSET $4`, q, owner, status, (page-1)*12)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	items := []adminBeadWork{}
	for rows.Next() {
		var item adminBeadWork
		var data []byte
		if err = rows.Scan(&item.ID, &item.Revision, &data, &item.UpdatedAt, &item.AccountID, &item.Username, &item.Nickname); err != nil {
			a.internal(w, err)
			return
		}
		if err = json.Unmarshal(data, &item.Data); err != nil {
			a.internal(w, err)
			return
		}
		item.Data.Target = []int{}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "total": total, "page": page, "page_size": 12})
}
