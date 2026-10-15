package main

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type slimProfile struct {
	Gender string  `json:"gender"`
	Height float64 `json:"height"`
}
type slimEntry struct {
	ID       string  `json:"id"`
	Day      string  `json:"day"`
	Kind     string  `json:"kind"`
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
	Rate     float64 `json:"rate"`
	Weight   float64 `json:"weight"`
	Calories float64 `json:"calories"`
}

const slimColumns = "id,day::text,kind,name,quantity,rate,weight,calories"

func scanSlim(row pgx.Row) (e slimEntry, err error) {
	err = row.Scan(&e.ID, &e.Day, &e.Kind, &e.Name, &e.Quantity, &e.Rate, &e.Weight, &e.Calories)
	return
}
func slimNumber(n, min, max float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0) && n >= min && n <= max
}
func prepareSlim(e *slimEntry, now time.Time) error {
	e.Name = strings.TrimSpace(e.Name)
	day, err := time.Parse("2006-01-02", e.Day)
	today := now.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
	if err != nil || day.Format("2006-01-02") != e.Day || e.Day > today || e.Day < "2020-01-01" {
		return errors.New("请选择2020年至今天之间的有效日期")
	}
	if len(e.ID) < 16 || len(e.ID) > 80 || strings.Trim(e.ID, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" {
		return errors.New("记录标识无效，请重新打开页面")
	}
	if e.Name == "" || utf8.RuneCountInString(e.Name) > 80 {
		return errors.New("名称请填写1至80字")
	}
	switch e.Kind {
	case "food":
		if !slimNumber(e.Quantity, 1, 5000) || !slimNumber(e.Rate, 0, 1000) {
			return errors.New("食物重量需为1–5000克，热量需为0–1000千卡/100克")
		}
		e.Weight = 0
		e.Calories = math.Round(e.Quantity*e.Rate/100*10) / 10
	case "exercise":
		mets := map[string]float64{"平地走路（约4.5–5公里/小时）": 3.5, "快走（约5.6–6.3公里/小时）": 4.8, "慢跑（自选配速）": 7.5, "休闲骑车（低于16公里/小时）": 4.0, "轻度拉伸": 2.3}
		met, ok := mets[e.Name]
		if !ok || !slimNumber(e.Quantity, 1, 600) || !slimNumber(e.Weight, 20, 400) {
			return errors.New("请选择运动，填写1–600分钟和20–400公斤的体重")
		}
		e.Rate = met
		e.Calories = math.Round(met*3.5*e.Weight/200*e.Quantity*10) / 10
	case "weight":
		if !slimNumber(e.Weight, 20, 400) {
			return errors.New("体重需为20–400公斤")
		}
		e.Quantity = 0
		e.Rate = 0
		e.Calories = 0
		e.Name = "体重打卡"
	default:
		return errors.New("记录类型无效")
	}
	return nil
}
func (a *app) slimList(w http.ResponseWriter, r *http.Request) {
	owner := r.Context().Value(guestKey)
	p := slimProfile{}
	err := a.db.QueryRow(r.Context(), "SELECT gender,height FROM slim_profiles WHERE account_id=$1", owner).Scan(&p.Gender, &p.Height)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		a.internal(w, err)
		return
	}
	rows, err := a.db.Query(r.Context(), "SELECT "+slimColumns+" FROM slim_entries WHERE account_id=$1 ORDER BY day DESC,created_at DESC,id DESC", owner)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	list := []slimEntry{}
	for rows.Next() {
		e, err := scanSlim(rows)
		if err != nil {
			a.internal(w, err)
			return
		}
		list = append(list, e)
	}
	if rows.Err() != nil {
		a.internal(w, rows.Err())
		return
	}
	jsonOut(w, 200, map[string]any{"profile": p, "entries": list})
}
func (a *app) slimSaveProfile(w http.ResponseWriter, r *http.Request) {
	var p slimProfile
	if !decode(w, r, &p) {
		return
	}
	if (p.Gender != "male" && p.Gender != "female") || !slimNumber(p.Height, 100, 250) {
		fail(w, 400, "请选择性别，身高需为100–250厘米")
		return
	}
	_, err := a.db.Exec(r.Context(), "INSERT INTO slim_profiles(account_id,gender,height) VALUES($1,$2,$3) ON CONFLICT(account_id) DO UPDATE SET gender=EXCLUDED.gender,height=EXCLUDED.height", r.Context().Value(guestKey), p.Gender, p.Height)
	if err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, p)
}
func (a *app) slimSave(w http.ResponseWriter, r *http.Request) {
	var e slimEntry
	if !decode(w, r, &e) {
		return
	}
	if err := prepareSlim(&e, time.Now()); err != nil {
		fail(w, 400, err.Error())
		return
	}
	owner := r.Context().Value(guestKey).(string)
	if !a.limits.allow("slim:"+owner, 60) {
		fail(w, 429, "记录太快啦，稍后再试")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var locked string
	if err = tx.QueryRow(r.Context(), "SELECT id FROM accounts WHERE id=$1 FOR UPDATE", owner).Scan(&locked); err != nil {
		a.internal(w, err)
		return
	}
	old, err := scanSlim(tx.QueryRow(r.Context(), "SELECT "+slimColumns+" FROM slim_entries WHERE account_id=$1 AND id=$2", owner, e.ID))
	if err == nil {
		if old != e {
			fail(w, 409, "记录已存在，请刷新后重试")
			return
		}
		jsonOut(w, 200, old)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		a.internal(w, err)
		return
	}
	var count int
	if err = tx.QueryRow(r.Context(), "SELECT count(*) FROM slim_entries WHERE account_id=$1", owner).Scan(&count); err != nil {
		a.internal(w, err)
		return
	}
	if count >= 5000 {
		fail(w, 409, "已达5000条，请先清理旧记录")
		return
	}
	_, err = tx.Exec(r.Context(), "INSERT INTO slim_entries(account_id,id,day,kind,name,quantity,rate,weight,calories) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)", owner, e.ID, e.Day, e.Kind, e.Name, e.Quantity, e.Rate, e.Weight, e.Calories)
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 201, e)
}
func (a *app) slimDelete(w http.ResponseWriter, r *http.Request) {
	result, err := a.db.Exec(r.Context(), "DELETE FROM slim_entries WHERE account_id=$1 AND id=$2", r.Context().Value(guestKey), r.PathValue("id"))
	if err != nil {
		a.internal(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		fail(w, 404, "记录不存在")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
