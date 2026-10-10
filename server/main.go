package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

//go:embed all:web
var assets embed.FS

type app struct {
	db            *pgxpool.Pool
	music         *musicClient
	adminPassword string
	limits        limiter
}
type Dish struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Emoji       string `json:"emoji"`
	Price       int    `json:"price"`
	Available   bool   `json:"available"`
}
type Line struct {
	DishID   int    `json:"dish_id"`
	Name     string `json:"name"`
	Emoji    string `json:"emoji"`
	Price    int    `json:"price"`
	Quantity int    `json:"quantity"`
	Mood     string `json:"mood"`
}
type Order struct {
	ID        string          `json:"id"`
	Number    int64           `json:"number"`
	Total     int             `json:"total"`
	Status    string          `json:"status"`
	Mood      string          `json:"mood"`
	Note      string          `json:"note"`
	Quote     string          `json:"quote"`
	CreatedAt time.Time       `json:"created_at"`
	Items     json.RawMessage `json:"items"`
}
type key string

const guestKey key = "guest"

var moods = []string{"勉强清醒", "灵魂离线", "已读乱回"}
var quotes = []string{"今天的 KPI：好好吃饭，准时下班。", "工作只是工作，你才是生活的主角。", "允许自己偶尔离线，世界不会断电。", "老板画的饼，记得让他自己吃。", "你已经很努力了，剩下的交给明天。", "本店盖章：今日内耗，到此为止。"}

func token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func hash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func choice(ss []string) string {
	n, e := rand.Int(rand.Reader, big.NewInt(int64(len(ss))))
	if e != nil {
		return ss[0]
	}
	return ss[n.Int64()]
}
func validMood(s string) bool {
	for _, v := range moods {
		if s == v {
			return true
		}
	}
	return false
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	jsonOut(w, status, map[string]string{"error": msg})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		fail(w, 400, "请求格式不正确")
		return false
	}
	if d.Decode(&struct{}{}) != io.EOF {
		fail(w, 400, "请求格式不正确")
		return false
	}
	return true
}
func bearer(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if !strings.HasPrefix(v, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(v, "Bearer ")
}
func (a *app) internal(w http.ResponseWriter, e error) {
	log.Printf("request failed: %v", e)
	fail(w, 500, "食堂暂时忙不过来，请稍后重试")
}

type bucket struct {
	count int
	until time.Time
}
type limiter struct {
	sync.Mutex
	entries map[string]bucket
}

func (l *limiter) allow(id string, n int) bool {
	l.Lock()
	defer l.Unlock()
	if l.entries == nil {
		l.entries = map[string]bucket{}
	}
	now := time.Now()
	b := l.entries[id]
	if now.After(b.until) {
		b = bucket{until: now.Add(time.Minute)}
	}
	b.count++
	l.entries[id] = b
	if len(l.entries) > 1000 {
		for k, v := range l.entries {
			if now.After(v.until) {
				delete(l.entries, k)
			}
		}
	}
	return b.count <= n
}
func clientIP(r *http.Request) string {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "127.0.0.1" || ip == "::1" {
		if v := r.Header.Get("X-Forwarded-For"); v != "" {
			parts := strings.Split(v, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	return ip
}
func (a *app) limited(prefix string, n int, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.limits.allow(prefix+clientIP(r), n) {
			fail(w, 429, "先喘口气，一分钟后再试")
			return
		}
		next(w, r)
	}
}
func (a *app) customer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := bearer(r)
		if len(t) != 64 {
			fail(w, 401, "请登录后继续")
			return
		}
		var id string
		e := a.db.QueryRow(r.Context(), "SELECT s.account_id FROM customer_sessions s JOIN accounts a ON a.id=s.account_id WHERE s.token_hash=$1 AND s.expires_at>now() AND NOT a.disabled", hash(t)).Scan(&id)
		if errors.Is(e, pgx.ErrNoRows) {
			fail(w, 401, "请登录后继续")
			return
		}
		if e != nil {
			a.internal(w, e)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), guestKey, id)))
	}
}
func (a *app) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		e := a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM admin_sessions WHERE token_hash=$1 AND expires_at>now())", hash(bearer(r))).Scan(&ok)
		if e != nil {
			a.internal(w, e)
			return
		}
		if !ok {
			fail(w, 401, "请先登录店长工作台")
			return
		}
		next(w, r)
	}
}
func (a *app) routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if e := a.db.Ping(ctx); e != nil {
			fail(w, 503, "数据库暂不可用")
			return
		}
		jsonOut(w, 200, map[string]string{"status": "ok"})
	})
	m.HandleFunc("POST /api/auth/register", a.limited("register:", 5, a.registerCustomer))
	m.HandleFunc("POST /api/auth/login", a.limited("customer-login:", 10, a.loginCustomer))
	m.HandleFunc("POST /api/auth/logout", a.customer(a.logoutCustomer))
	m.HandleFunc("GET /api/music/{provider}", a.withMusicProvider(a.customer(a.musicStatus)))
	m.HandleFunc("POST /api/music/{provider}/qr", a.withMusicProvider(a.customer(a.limited("music-start-ip:", 10, a.musicStart))))
	m.HandleFunc("POST /api/music/{provider}/qr/check", a.withMusicProvider(a.customer(a.musicCheck)))
	m.HandleFunc("POST /api/music/{provider}/qr/cancel", a.withMusicProvider(a.customer(a.musicCancel)))
	m.HandleFunc("POST /api/music/{provider}/unbind", a.withMusicProvider(a.customer(a.musicUnbind)))
	m.HandleFunc("GET /api/music/{provider}/playlists", a.withMusicProvider(a.customer(a.musicPlaylists)))
	m.HandleFunc("GET /api/music/{provider}/tracks", a.withMusicProvider(a.customer(a.musicTracks)))
	m.HandleFunc("POST /api/music/{provider}/playback", a.withMusicProvider(a.customer(a.musicPlayback)))
	m.HandleFunc("GET /api/menu", a.menu)
	m.HandleFunc("GET /api/me", a.customer(a.me))
	m.HandleFunc("POST /api/claim", a.customer(a.claim))
	m.HandleFunc("GET /api/orders", a.customer(a.orders))
	m.HandleFunc("POST /api/orders", a.customer(a.limited("order:", 30, a.createOrder)))
	m.HandleFunc("POST /api/admin/login", a.limited("login:", 5, a.login))
	m.HandleFunc("POST /api/admin/logout", a.admin(a.logout))
	m.HandleFunc("GET /api/admin/orders", a.admin(a.adminOrders))
	m.HandleFunc("GET /api/admin/summary", a.admin(a.summary))
	m.HandleFunc("GET /api/admin/overview", a.admin(a.overview))
	m.HandleFunc("GET /api/admin/customers", a.admin(a.customers))
	m.HandleFunc("GET /api/admin/customers/{id}", a.admin(a.customerDetail))
	m.HandleFunc("PATCH /api/admin/customers/{id}", a.admin(a.updateCustomer))
	m.HandleFunc("GET /api/admin/order-list", a.admin(a.adminOrderList))
	m.HandleFunc("PATCH /api/admin/orders/{id}", a.admin(a.updateOrder))
	m.HandleFunc("POST /api/admin/dishes", a.admin(a.saveDish))
	m.HandleFunc("PUT /api/admin/dishes/{id}", a.admin(a.saveDish))
	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "接口不存在") })
	web, _ := fs.Sub(assets, "web")
	m.Handle("/", http.FileServer(http.FS(web)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		m.ServeHTTP(w, r.WithContext(ctx))
	})
}
func (a *app) me(w http.ResponseWriter, r *http.Request) {
	var balance int
	var claimed bool
	var id, username string
	e := a.db.QueryRow(r.Context(), "SELECT g.id,a.username,g.balance,COALESCE(g.last_claim=(now() AT TIME ZONE 'Asia/Shanghai')::date,false) FROM guests g JOIN accounts a ON a.id=g.id WHERE g.id=$1", r.Context().Value(guestKey)).Scan(&id, &username, &balance, &claimed)
	if e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 200, map[string]any{"id": id, "username": username, "balance": balance, "claimed_today": claimed})
}
func (a *app) claim(w http.ResponseWriter, r *http.Request) {
	_, e := a.db.Exec(r.Context(), "UPDATE guests SET balance=balance+100,last_claim=(now() AT TIME ZONE 'Asia/Shanghai')::date WHERE id=$1 AND (last_claim IS NULL OR last_claim<(now() AT TIME ZONE 'Asia/Shanghai')::date)", r.Context().Value(guestKey))
	if e != nil {
		a.internal(w, e)
		return
	}
	a.me(w, r)
}
func (a *app) menu(w http.ResponseWriter, r *http.Request) {
	rows, e := a.db.Query(r.Context(), "SELECT id,name,description,category,emoji,price,available FROM dishes ORDER BY id")
	if e != nil {
		a.internal(w, e)
		return
	}
	defer rows.Close()
	ds := []Dish{}
	for rows.Next() {
		var d Dish
		if e = rows.Scan(&d.ID, &d.Name, &d.Description, &d.Category, &d.Emoji, &d.Price, &d.Available); e != nil {
			a.internal(w, e)
			return
		}
		ds = append(ds, d)
	}
	if rows.Err() != nil {
		a.internal(w, rows.Err())
		return
	}
	jsonOut(w, 200, ds)
}

const orderCols = "id,number,total,status,mood,note,quote,created_at,items"

func scanOrder(row pgx.Row) (Order, error) {
	var o Order
	e := row.Scan(&o.ID, &o.Number, &o.Total, &o.Status, &o.Mood, &o.Note, &o.Quote, &o.CreatedAt, &o.Items)
	return o, e
}
func (a *app) listOrders(w http.ResponseWriter, r *http.Request, admin bool) {
	q := "SELECT " + orderCols + " FROM orders"
	args := []any{}
	if !admin {
		q += " WHERE guest_id=$1"
		args = append(args, r.Context().Value(guestKey))
	}
	q += " ORDER BY created_at DESC LIMIT 100"
	rows, e := a.db.Query(r.Context(), q, args...)
	if e != nil {
		a.internal(w, e)
		return
	}
	defer rows.Close()
	out := []Order{}
	for rows.Next() {
		o, e := scanOrder(rows)
		if e != nil {
			a.internal(w, e)
			return
		}
		out = append(out, o)
	}
	if rows.Err() != nil {
		a.internal(w, rows.Err())
		return
	}
	jsonOut(w, 200, out)
}
func (a *app) orders(w http.ResponseWriter, r *http.Request)      { a.listOrders(w, r, false) }
func (a *app) adminOrders(w http.ResponseWriter, r *http.Request) { a.listOrders(w, r, true) }

type orderInput struct {
	RequestKey string `json:"request_key"`
	Mood       string `json:"mood"`
	Note       string `json:"note"`
	Items      []struct {
		DishID   int    `json:"dish_id"`
		Quantity int    `json:"quantity"`
		Mood     string `json:"mood"`
	} `json:"items"`
}

func validateOrder(in orderInput) bool {
	if len(in.RequestKey) < 16 || len(in.RequestKey) > 100 || !validMood(in.Mood) || utf8.RuneCountInString(in.Note) > 100 || len(in.Items) < 1 || len(in.Items) > 20 {
		return false
	}
	sum := 0
	for _, it := range in.Items {
		if it.DishID < 1 || it.Quantity < 1 || it.Quantity > 10 || !validMood(it.Mood) {
			return false
		}
		sum += it.Quantity
	}
	return sum <= 30
}
func (a *app) createOrder(w http.ResponseWriter, r *http.Request) {
	var in orderInput
	if !decode(w, r, &in) {
		return
	}
	if !validateOrder(in) {
		fail(w, 400, "请检查菜品数量、精神状态和备注（最多100字）")
		return
	}
	ctx := r.Context()
	gid := ctx.Value(guestKey)
	tx, e := a.db.Begin(ctx)
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	var balance int
	e = tx.QueryRow(ctx, "SELECT balance FROM guests WHERE id=$1 FOR UPDATE", gid).Scan(&balance)
	if e != nil {
		a.internal(w, e)
		return
	}
	existing, e := scanOrder(tx.QueryRow(ctx, "SELECT "+orderCols+" FROM orders WHERE guest_id=$1 AND request_key=$2", gid, in.RequestKey))
	if e == nil {
		jsonOut(w, 200, existing)
		return
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		a.internal(w, e)
		return
	}
	total := 0
	items := []Line{}
	for _, i := range in.Items {
		var d Dish
		e = tx.QueryRow(ctx, "SELECT id,name,emoji,price,available FROM dishes WHERE id=$1 FOR SHARE", i.DishID).Scan(&d.ID, &d.Name, &d.Emoji, &d.Price, &d.Available)
		if errors.Is(e, pgx.ErrNoRows) {
			fail(w, 400, "有菜品已下架，请刷新菜单")
			return
		}
		if e != nil {
			a.internal(w, e)
			return
		}
		if !d.Available {
			fail(w, 409, d.Name+" 已售罄，请重新选餐")
			return
		}
		total += d.Price * i.Quantity
		items = append(items, Line{d.ID, d.Name, d.Emoji, d.Price, i.Quantity, i.Mood})
	}
	if total > balance {
		fail(w, 409, "精神值不足，先领取今日补给或少点一些")
		return
	}
	raw, _ := json.Marshal(items)
	o, e := scanOrder(tx.QueryRow(ctx, "INSERT INTO orders(id,guest_id,request_key,total,mood,note,quote,items) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING "+orderCols, token(), gid, in.RequestKey, total, in.Mood, in.Note, choice(quotes), raw))
	if e != nil {
		a.internal(w, e)
		return
	}
	_, e = tx.Exec(ctx, "UPDATE guests SET balance=balance-$1 WHERE id=$2", total, gid)
	if e != nil {
		a.internal(w, e)
		return
	}
	if e = tx.Commit(ctx); e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 201, o)
}
func (a *app) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	x, y := sha256.Sum256([]byte(in.Password)), sha256.Sum256([]byte(a.adminPassword))
	if subtle.ConstantTimeCompare(x[:], y[:]) != 1 {
		fail(w, 401, "店长口令不正确")
		return
	}
	t := token()
	_, e := a.db.Exec(r.Context(), "INSERT INTO admin_sessions(token_hash,expires_at) VALUES($1,now()+interval '12 hours')", hash(t))
	if e != nil {
		a.internal(w, e)
		return
	}
	_, _ = a.db.Exec(r.Context(), "DELETE FROM admin_sessions WHERE expires_at<now()")
	jsonOut(w, 200, map[string]string{"token": t})
}
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	_, e := a.db.Exec(r.Context(), "DELETE FROM admin_sessions WHERE token_hash=$1", hash(bearer(r)))
	if e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func canTransition(from, to string) bool {
	return (from == "pending" && (to == "cooking" || to == "cancelled")) || (from == "cooking" && (to == "ready" || to == "cancelled")) || (from == "ready" && to == "completed")
}
func (a *app) updateOrder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	tx, e := a.db.Begin(ctx)
	if e != nil {
		a.internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	var status, gid string
	var total int
	e = tx.QueryRow(ctx, "SELECT status,guest_id,total FROM orders WHERE id=$1 FOR UPDATE", r.PathValue("id")).Scan(&status, &gid, &total)
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 404, "订单不存在")
		return
	}
	if e != nil {
		a.internal(w, e)
		return
	}
	if !canTransition(status, in.Status) {
		fail(w, 409, "订单状态已经变化，请刷新后再试")
		return
	}
	if in.Status == "cancelled" {
		_, e = tx.Exec(ctx, "UPDATE guests SET balance=balance+$1 WHERE id=$2", total, gid)
		if e != nil {
			a.internal(w, e)
			return
		}
	}
	_, e = tx.Exec(ctx, "UPDATE orders SET status=$1,updated_at=now() WHERE id=$2", in.Status, r.PathValue("id"))
	if e != nil {
		a.internal(w, e)
		return
	}
	if e = tx.Commit(ctx); e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func validDish(d Dish) bool {
	if strings.TrimSpace(d.Name) == "" || utf8.RuneCountInString(d.Name) > 30 || utf8.RuneCountInString(d.Description) > 120 || utf8.RuneCountInString(d.Emoji) > 8 || d.Emoji == "" || d.Price < 1 || d.Price > 200 {
		return false
	}
	for _, c := range []string{"续命主食", "精神饮品", "摸鱼小食", "离职套餐"} {
		if d.Category == c {
			return true
		}
	}
	return false
}
func (a *app) saveDish(w http.ResponseWriter, r *http.Request) {
	var d Dish
	if !decode(w, r, &d) {
		return
	}
	if !validDish(d) {
		fail(w, 400, "菜品名称、分类、图标或精神值不正确（1—200）")
		return
	}
	if r.Method == "POST" {
		e := a.db.QueryRow(r.Context(), "INSERT INTO dishes(name,description,category,emoji,price,available) VALUES($1,$2,$3,$4,$5,$6) RETURNING id", d.Name, d.Description, d.Category, d.Emoji, d.Price, d.Available).Scan(&d.ID)
		if e != nil {
			a.internal(w, e)
			return
		}
		jsonOut(w, 201, d)
		return
	}
	result, e := a.db.Exec(r.Context(), "UPDATE dishes SET name=$1,description=$2,category=$3,emoji=$4,price=$5,available=$6 WHERE id::text=$7", d.Name, d.Description, d.Category, d.Emoji, d.Price, d.Available, r.PathValue("id"))
	if e != nil {
		a.internal(w, e)
		return
	}
	if result.RowsAffected() == 0 {
		fail(w, 404, "菜品不存在")
		return
	}
	jsonOut(w, 200, d)
}
func (a *app) summary(w http.ResponseWriter, r *http.Request) {
	var orders, pending, points int
	e := a.db.QueryRow(r.Context(), "SELECT count(*),count(*) FILTER(WHERE status IN ('pending','cooking','ready')),COALESCE(sum(total) FILTER(WHERE status<>'cancelled'),0) FROM orders WHERE created_at>=(date_trunc('day',now() AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai')").Scan(&orders, &pending, &points)
	if e != nil {
		a.internal(w, e)
		return
	}
	jsonOut(w, 200, map[string]int{"orders": orders, "active": pending, "points": points})
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		c := http.Client{Timeout: 3 * time.Second}
		addr := os.Getenv("LISTEN_ADDR")
		if addr == "" {
			addr = "127.0.0.1:18082"
		}
		resp, e := c.Get("http://" + addr + "/api/health")
		if e != nil {
			os.Exit(1)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	dsn := os.Getenv("DATABASE_URL")
	pw := os.Getenv("ADMIN_PASSWORD")
	if dsn == "" || len(pw) < 6 {
		log.Fatal("DATABASE_URL and ADMIN_PASSWORD (6+ characters) are required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		log.Fatal("invalid database configuration")
	}
	cfg.MaxConns = 5
	cfg.MinConns = 0
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		log.Fatal("database initialization failed")
	}
	defer db.Close()
	if e = db.Ping(ctx); e != nil {
		log.Fatal("database unavailable")
	}
	if _, e = db.Exec(ctx, schema); e != nil {
		log.Fatalf("schema migration failed: %v", e)
	}
	music, e := configureMusic()
	if e != nil {
		log.Fatal("invalid music service configuration")
	}
	a := &app{db: db, adminPassword: pw, music: music}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:18082"
	}
	srv := &http.Server{Addr: addr, Handler: a.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-done
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	fmt.Println("精神离职 listening on", addr)
	if e = srv.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
		log.Fatal(e)
	}
}
