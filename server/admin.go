package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const pageSize = 20

type Customer struct {
	ID             string     `json:"id"`
	Username       string     `json:"username"`
	Balance        int        `json:"balance"`
	Disabled       bool       `json:"disabled"`
	AdminNote      string     `json:"admin_note"`
	CreatedAt      time.Time  `json:"created_at"`
	LastLoginAt    *time.Time `json:"last_login_at"`
	LastClaim      *string    `json:"last_claim"`
	OrderCount     int        `json:"order_count"`
	CompletedCount int        `json:"completed_count"`
	CancelledCount int        `json:"cancelled_count"`
	ActiveCount    int        `json:"active_count"`
	TotalPoints    int        `json:"total_points"`
	LastOrderAt    *time.Time `json:"last_order_at"`
}

// Explicit projection: never return credential hashes or session tokens to the admin UI.
const customerSelect = `SELECT a.id,a.username,g.balance,a.disabled,a.admin_note,a.created_at,a.last_login_at,
 to_char(g.last_claim,'YYYY-MM-DD'),s.order_count,s.completed_count,s.cancelled_count,s.active_count,s.total_points,s.last_order_at
 FROM accounts a JOIN guests g ON g.id=a.id
 LEFT JOIN LATERAL (SELECT count(*)::int order_count,
 count(*) FILTER(WHERE status='completed')::int completed_count,
 count(*) FILTER(WHERE status='cancelled')::int cancelled_count,
 count(*) FILTER(WHERE status IN ('pending','cooking','ready'))::int active_count,
 COALESCE(sum(total) FILTER(WHERE status<>'cancelled'),0)::int total_points,max(created_at) last_order_at
 FROM orders WHERE guest_id=a.id) s ON true`

func scanCustomer(row pgx.Row) (Customer, error) {
	var c Customer
	e := row.Scan(&c.ID, &c.Username, &c.Balance, &c.Disabled, &c.AdminNote, &c.CreatedAt, &c.LastLoginAt, &c.LastClaim, &c.OrderCount, &c.CompletedCount, &c.CancelledCount, &c.ActiveCount, &c.TotalPoints, &c.LastOrderAt)
	return c, e
}
func pagination(w http.ResponseWriter, r *http.Request) (int, bool) {
	v := r.URL.Query().Get("page")
	if v == "" {
		return 1, true
	}
	n, e := strconv.Atoi(v)
	if e != nil || n < 1 || n > 1000000 {
		fail(w, 400, "页码不正确")
		return 0, false
	}
	return n, true
}
func searchQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) > 80 {
		fail(w, 400, "搜索内容最多80个字符")
		return "", false
	}
	return q, true
}
func (a *app) customers(w http.ResponseWriter, r *http.Request) {
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
	if status != "all" && status != "enabled" && status != "disabled" {
		fail(w, 400, "账号状态不正确")
		return
	}
	where := ` WHERE ($1='' OR strpos(a.username,lower($1))>0 OR a.id=$1) AND ($2='all' OR ($2='disabled' AND a.disabled) OR ($2='enabled' AND NOT a.disabled))`
	var total int
	e := a.db.QueryRow(r.Context(), "SELECT count(*) FROM accounts a"+where, q, status).Scan(&total)
	if e != nil {
		a.internal(w, e)
		return
	}
	rows, e := a.db.Query(r.Context(), customerSelect+where+" ORDER BY a.created_at DESC,a.id LIMIT $3 OFFSET $4", q, status, pageSize, (page-1)*pageSize)
	if e != nil {
		a.internal(w, e)
		return
	}
	defer rows.Close()
	items := []Customer{}
	for rows.Next() {
		c, e := scanCustomer(rows)
		if e != nil {
			a.internal(w, e)
			return
		}
		items = append(items, c)
	}
	if rows.Err() != nil {
		a.internal(w, rows.Err())
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize})
}

type PopularDish struct {
	Name     string `json:"name"`
	Emoji    string `json:"emoji"`
	Quantity int    `json:"quantity"`
}

func scanPopular(rows pgx.Rows) ([]PopularDish, error) {
	defer rows.Close()
	items := []PopularDish{}
	for rows.Next() {
		var d PopularDish
		if e := rows.Scan(&d.Name, &d.Emoji, &d.Quantity); e != nil {
			return nil, e
		}
		items = append(items, d)
	}
	return items, rows.Err()
}
func (a *app) customerDetail(w http.ResponseWriter, r *http.Request) {
	c, e := scanCustomer(a.db.QueryRow(r.Context(), customerSelect+" WHERE a.id=$1", r.PathValue("id")))
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 404, "顾客不存在")
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	rows, e := a.db.Query(r.Context(), `SELECT item->>'name',COALESCE(item->>'emoji','🍽️'),sum((item->>'quantity')::int)::int FROM orders o CROSS JOIN LATERAL jsonb_array_elements(o.items) item WHERE o.guest_id=$1 AND o.status<>'cancelled' GROUP BY 1,2 ORDER BY 3 DESC,1 LIMIT 5`, c.ID)
	if e != nil {
		a.internal(w, e)
		return
	}
	popular, e := scanPopular(rows)
	if e != nil {
		a.internal(w, e)
		return
	}
	var mood string
	e = a.db.QueryRow(r.Context(), "SELECT COALESCE((SELECT mood FROM orders WHERE guest_id=$1 AND status<>'cancelled' GROUP BY mood ORDER BY count(*) DESC,mood LIMIT 1),'')", c.ID).Scan(&mood)
	if e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 200, map[string]any{"customer": c, "favorites": popular, "favorite_mood": mood})
}
func (a *app) updateCustomer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Disabled  *bool   `json:"disabled"`
		AdminNote *string `json:"admin_note"`
	}
	if !decode(w, r, &in) {
		return
	}
	if (in.Disabled == nil && in.AdminNote == nil) || (in.AdminNote != nil && utf8.RuneCountInString(*in.AdminNote) > 500) {
		fail(w, 400, "请填写账号状态或500字以内的备注")
		return
	}
	ctx := r.Context()
	tx, e := a.db.Begin(ctx)
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	var existing bool
	e = tx.QueryRow(ctx, "SELECT disabled FROM accounts WHERE id=$1 FOR UPDATE", r.PathValue("id")).Scan(&existing)
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 404, "顾客不存在")
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	_, e = tx.Exec(ctx, "UPDATE accounts SET disabled=COALESCE($1,disabled),admin_note=COALESCE($2,admin_note) WHERE id=$3", in.Disabled, in.AdminNote, r.PathValue("id"))
	if e != nil {
		a.internal(w, e)
		return
	}
	if in.Disabled != nil && *in.Disabled {
		_, e = tx.Exec(ctx, "DELETE FROM customer_sessions WHERE account_id=$1", r.PathValue("id"))
		if e != nil {
			a.internal(w, e)
			return
		}
	}
	if e = tx.Commit(ctx); e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

type AdminOrder struct {
	Order
	CustomerID string    `json:"customer_id"`
	Username   string    `json:"username"`
	UpdatedAt  time.Time `json:"updated_at"`
}

const adminOrderColumns = "o.id,o.number,o.total,o.status,o.mood,o.note,o.quote,o.created_at,o.items,COALESCE(a.id,''),COALESCE(a.username,'历史访客'),o.updated_at"

func scanAdminOrder(row pgx.Row) (AdminOrder, error) {
	var o AdminOrder
	e := row.Scan(&o.ID, &o.Number, &o.Total, &o.Status, &o.Mood, &o.Note, &o.Quote, &o.CreatedAt, &o.Items, &o.CustomerID, &o.Username, &o.UpdatedAt)
	return o, e
}
func (a *app) adminOrderList(w http.ResponseWriter, r *http.Request) {
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
	switch status {
	case "all", "active", "pending", "cooking", "ready", "completed", "cancelled":
	default:
		fail(w, 400, "订单状态不正确")
		return
	}
	if n, e := strconv.ParseInt(strings.TrimPrefix(strings.ToUpper(q), "MO-"), 10, 64); e == nil {
		q = strconv.FormatInt(n, 10)
	}
	cid := r.URL.Query().Get("customer_id")
	if len(cid) > 64 {
		fail(w, 400, "顾客编号不正确")
		return
	}
	from := ` FROM orders o LEFT JOIN accounts a ON a.id=o.guest_id WHERE ($1='' OR strpos(COALESCE(a.username,'历史访客'),lower($1))>0 OR o.number::text=$1)
 AND ($2='all' OR ($2='active' AND o.status IN ('pending','cooking','ready')) OR o.status=$2) AND ($3='' OR a.id=$3)`
	var total int
	e := a.db.QueryRow(r.Context(), "SELECT count(*)"+from, q, status, cid).Scan(&total)
	if e != nil {
		a.internal(w, e)
		return
	}
	rows, e := a.db.Query(r.Context(), "SELECT "+adminOrderColumns+from+" ORDER BY o.created_at DESC,o.number DESC LIMIT $4 OFFSET $5", q, status, cid, pageSize, (page-1)*pageSize)
	if e != nil {
		a.internal(w, e)
		return
	}
	defer rows.Close()
	items := []AdminOrder{}
	for rows.Next() {
		o, e := scanAdminOrder(rows)
		if e != nil {
			a.internal(w, e)
			return
		}
		items = append(items, o)
	}
	if rows.Err() != nil {
		a.internal(w, rows.Err())
		return
	}
	jsonOut(w, 200, map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize})
}
func (a *app) overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var today, total, pending, cooking, ready, completed, cancelled, points, customers, newCustomers, disabled, available int
	e := a.db.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE (created_at AT TIME ZONE 'Asia/Shanghai')::date=(now() AT TIME ZONE 'Asia/Shanghai')::date),
 count(*) FILTER(WHERE status='pending'),count(*) FILTER(WHERE status='cooking'),count(*) FILTER(WHERE status='ready'),count(*) FILTER(WHERE status='completed'),count(*) FILTER(WHERE status='cancelled'),
 COALESCE(sum(total) FILTER(WHERE status<>'cancelled' AND (created_at AT TIME ZONE 'Asia/Shanghai')::date=(now() AT TIME ZONE 'Asia/Shanghai')::date),0) FROM orders`).Scan(&total, &today, &pending, &cooking, &ready, &completed, &cancelled, &points)
	if e != nil {
		a.internal(w, e)
		return
	}
	e = a.db.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE (created_at AT TIME ZONE 'Asia/Shanghai')::date=(now() AT TIME ZONE 'Asia/Shanghai')::date),count(*) FILTER(WHERE disabled) FROM accounts`).Scan(&customers, &newCustomers, &disabled)
	if e != nil {
		a.internal(w, e)
		return
	}
	e = a.db.QueryRow(ctx, "SELECT count(*) FROM dishes WHERE available").Scan(&available)
	if e != nil {
		a.internal(w, e)
		return
	}
	type Day struct {
		Date   string `json:"date"`
		Orders int    `json:"orders"`
		Points int    `json:"points"`
	}
	trend := []Day{}
	rows, e := a.db.Query(ctx, `WITH dates AS (SELECT generate_series((now() AT TIME ZONE 'Asia/Shanghai')::date-6,(now() AT TIME ZONE 'Asia/Shanghai')::date,interval '1 day')::date AS day) SELECT to_char(d.day,'YYYY-MM-DD'),count(o.id)::int,COALESCE(sum(o.total) FILTER(WHERE o.status<>'cancelled'),0)::int FROM dates d LEFT JOIN orders o ON (o.created_at AT TIME ZONE 'Asia/Shanghai')::date=d.day GROUP BY d.day ORDER BY d.day`)
	if e != nil {
		a.internal(w, e)
		return
	}
	for rows.Next() {
		var d Day
		if e = rows.Scan(&d.Date, &d.Orders, &d.Points); e != nil {
			rows.Close()
			a.internal(w, e)
			return
		}
		trend = append(trend, d)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		a.internal(w, e)
		return
	}
	rows, e = a.db.Query(ctx, `SELECT item->>'name',COALESCE(item->>'emoji','🍽️'),sum((item->>'quantity')::int)::int FROM orders o CROSS JOIN LATERAL jsonb_array_elements(o.items) item WHERE o.status<>'cancelled' AND o.created_at>=(((now() AT TIME ZONE 'Asia/Shanghai')::date-6)::timestamp AT TIME ZONE 'Asia/Shanghai') GROUP BY 1,2 ORDER BY 3 DESC,1 LIMIT 5`)
	if e != nil {
		a.internal(w, e)
		return
	}
	popular, e := scanPopular(rows)
	if e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 200, map[string]any{"today_orders": today, "total_orders": total, "pending": pending, "cooking": cooking, "ready": ready, "completed": completed, "cancelled": cancelled, "today_points": points, "customers": customers, "new_customers": newCustomers, "disabled_customers": disabled, "available_dishes": available, "trend": trend, "popular": popular})
}
