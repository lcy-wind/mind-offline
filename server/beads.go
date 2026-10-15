package main

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type beadData struct {
	Name     string `json:"name"`
	Size     int    `json:"size"`
	Mode     string `json:"mode"`
	Pixels   []int  `json:"pixels"`
	Target   []int  `json:"target"`
	Finished bool   `json:"finished"`
}
type beadWork struct {
	ID        string    `json:"id"`
	Revision  int       `json:"revision"`
	Data      beadData  `json:"data"`
	UpdatedAt time.Time `json:"updated_at"`
}

func validateBeads(d *beadData) bool {
	d.Name = strings.TrimSpace(d.Name)
	if utf8.RuneCountInString(d.Name) < 1 || utf8.RuneCountInString(d.Name) > 40 || (d.Size != 16 && d.Size != 32 && d.Size != 48) || (d.Mode != "free" && d.Mode != "guided") || len(d.Pixels) != d.Size*d.Size {
		return false
	}
	if d.Mode == "guided" && len(d.Target) != len(d.Pixels) {
		return false
	}
	if d.Mode == "free" && len(d.Target) != 0 {
		return false
	}
	for _, a := range [][]int{d.Pixels, d.Target} {
		for _, v := range a {
			if v < 0 || v > 32 {
				return false
			}
		}
	}
	filled, total, correct, extra := 0, 0, 0, 0
	for i, v := range d.Pixels {
		if v > 0 {
			filled++
		}
		if d.Mode == "guided" {
			if d.Target[i] > 0 {
				total++
				if v == d.Target[i] {
					correct++
				}
			} else if v > 0 {
				extra++
			}
		}
	}
	if d.Mode == "guided" && total == 0 {
		return false
	}
	if d.Finished && (filled == 0 || (d.Mode == "guided" && (correct != total || extra > 0))) {
		return false
	}
	if d.Target == nil {
		d.Target = []int{}
	}
	return true
}
func scanBeads(row pgx.Row) (b beadWork, err error) {
	var data []byte
	err = row.Scan(&b.ID, &b.Revision, &data, &b.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(data, &b.Data)
	}
	return
}

const beadColumns = "id,revision,data,updated_at"

func (a *app) beadList(w http.ResponseWriter, r *http.Request) {
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		var e error
		page, e = strconv.Atoi(p)
		if e != nil || page < 1 || page > 9 {
			fail(w, 400, "页码不正确")
			return
		}
	}
	owner := r.Context().Value(guestKey)
	var total int
	if err := a.db.QueryRow(r.Context(), "SELECT count(*) FROM bead_works WHERE account_id=$1", owner).Scan(&total); err != nil {
		a.internal(w, err)
		return
	}
	rows, err := a.db.Query(r.Context(), "SELECT "+beadColumns+" FROM bead_works WHERE account_id=$1 ORDER BY updated_at DESC,id LIMIT 12 OFFSET $2", owner, (page-1)*12)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	items := []beadWork{}
	for rows.Next() {
		b, e := scanBeads(rows)
		if e != nil {
			a.internal(w, e)
			return
		}
		items = append(items, b)
	}
	if rows.Err() != nil {
		a.internal(w, rows.Err())
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "total": total})
}
func (a *app) beadGet(w http.ResponseWriter, r *http.Request) {
	b, e := scanBeads(a.db.QueryRow(r.Context(), "SELECT "+beadColumns+" FROM bead_works WHERE account_id=$1 AND id=$2", r.Context().Value(guestKey), r.PathValue("id")))
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 404, "作品不存在或已删除")
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 200, b)
}
func (a *app) beadSave(w http.ResponseWriter, r *http.Request) {
	var b beadWork
	if !decode(w, r, &b) {
		return
	}
	if len(b.ID) < 16 || len(b.ID) > 80 || strings.Trim(b.ID, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" || b.Revision < 0 || !validateBeads(&b.Data) {
		fail(w, 400, "作品数据无效，请检查名称、画布和颜色")
		return
	}
	owner := r.Context().Value(guestKey).(string)
	if !a.limits.allow("bead-save:"+owner, 90) {
		fail(w, 429, "保存太快啦，请稍后再试")
		return
	}
	data, _ := json.Marshal(b.Data)
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var lock string
	if err = tx.QueryRow(r.Context(), "SELECT id FROM accounts WHERE id=$1 FOR UPDATE", owner).Scan(&lock); err != nil {
		a.internal(w, err)
		return
	}
	existing, err := scanBeads(tx.QueryRow(r.Context(), "SELECT "+beadColumns+" FROM bead_works WHERE account_id=$1 AND id=$2 FOR UPDATE", owner, b.ID))
	if err == nil {
		old, _ := json.Marshal(existing.Data)
		if string(old) == string(data) {
			jsonOut(w, 200, existing)
			return
		}
		if existing.Revision != b.Revision {
			fail(w, 409, "这件作品已在其他页面修改，本页草稿保留，可另存一份")
			return
		}
		b, err = scanBeads(tx.QueryRow(r.Context(), "UPDATE bead_works SET data=$3,revision=revision+1,updated_at=clock_timestamp() WHERE account_id=$1 AND id=$2 RETURNING "+beadColumns, owner, b.ID, data))
	} else if errors.Is(err, pgx.ErrNoRows) {
		if b.Revision != 0 {
			fail(w, 409, "云端作品已删除，本页草稿可另存一份")
			return
		}
		var count int
		err = tx.QueryRow(r.Context(), "SELECT count(*) FROM bead_works WHERE account_id=$1", owner).Scan(&count)
		if err != nil {
			a.internal(w, err)
			return
		}
		if count >= 100 {
			fail(w, 409, "最多保存100件作品，请先整理作品库")
			return
		}
		b, err = scanBeads(tx.QueryRow(r.Context(), "INSERT INTO bead_works(account_id,id,data) VALUES($1,$2,$3) RETURNING "+beadColumns, owner, b.ID, data))
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, b)
}
func (a *app) beadDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	res, err := a.db.Exec(r.Context(), "DELETE FROM bead_works WHERE account_id=$1 AND id=$2 AND revision=$3", r.Context().Value(guestKey), r.PathValue("id"), in.Revision)
	if err != nil {
		a.internal(w, err)
		return
	}
	if res.RowsAffected() == 0 {
		fail(w, 409, "作品已变化或不存在，请刷新作品库后再操作")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
