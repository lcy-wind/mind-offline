package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"log"
	"net/http"
	"strings"
	"time"
)

type gomokuPlayer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}
type gomokuMove struct {
	X    int `json:"x"`
	Y    int `json:"y"`
	Side int `json:"side"`
}
type gomokuGame struct {
	ID       string          `json:"id"`
	Status   string          `json:"status"`
	Stake    int             `json:"stake"`
	Players  [2]gomokuPlayer `json:"players"`
	Board    [225]int        `json:"board"`
	Moves    []gomokuMove    `json:"moves"`
	Turn     int             `json:"turn"`
	Winner   int             `json:"winner"`
	Reason   string          `json:"reason"`
	Deadline time.Time       `json:"deadline"`
}
type gomokuBet struct {
	AccountID string `json:"account_id"`
	Lane      string `json:"lane"`
	Side      int    `json:"side"`
	Amount    int    `json:"amount"`
	Payout    int    `json:"payout"`
	Settled   bool   `json:"settled"`
}
type gomokuInput struct {
	RequestID string `json:"request_id"`
	Action    string `json:"action"`
	GameID    string `json:"game_id"`
	Amount    int    `json:"amount"`
	Side      int    `json:"side"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Ply       int    `json:"ply"`
}

func (g *gomokuGame) side(owner string) int {
	for i, p := range g.Players {
		if p.ID == owner {
			return i + 1
		}
	}
	return 0
}
func gomokuWon(board [225]int, x, y, side int) bool {
	for _, d := range [][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}} {
		n := 1
		for _, sign := range []int{-1, 1} {
			for k := 1; k < 15; k++ {
				xx, yy := x+d[0]*k*sign, y+d[1]*k*sign
				if xx < 0 || xx >= 15 || yy < 0 || yy >= 15 || board[yy*15+xx] != side {
					break
				}
				n++
			}
		}
		if n >= 5 {
			return true
		}
	}
	return false
}
func gomokuLock(ctx context.Context, tx pgx.Tx) (*gomokuGame, error) {
	var id *string
	if err := tx.QueryRow(ctx, "SELECT game_id FROM gomoku_table WHERE id=1 FOR UPDATE").Scan(&id); err != nil {
		return nil, err
	}
	if id == nil {
		return nil, nil
	}
	var g gomokuGame
	err := tx.QueryRow(ctx, "SELECT data FROM gomoku_games WHERE id=$1", *id).Scan(&g)
	return &g, err
}
func gomokuSave(ctx context.Context, tx pgx.Tx, g *gomokuGame) error {
	_, err := tx.Exec(ctx, "UPDATE gomoku_games SET data=$2,updated_at=clock_timestamp() WHERE id=$1", g.ID, g)
	return err
}

var errGomokuBalance = errors.New("精神值不足，换个小一点的押注吧")

func gomokuMoney(ctx context.Context, tx pgx.Tx, game, owner, lane, key string, delta int) error {
	result, err := tx.Exec(ctx, "INSERT INTO gomoku_ledger(game_id,account_id,lane,event_key,delta) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING", game, owner, lane, key, delta)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return nil
	}
	result, err = tx.Exec(ctx, "UPDATE guests SET balance=balance+$2 WHERE id=$1 AND balance+$2>=0", owner, delta)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errGomokuBalance
	}
	return nil
}
func gomokuSettle(ctx context.Context, tx pgx.Tx, g *gomokuGame, winner int, reason string) error {
	if g.Status == "ended" {
		return nil
	}
	rows, err := tx.Query(ctx, "SELECT account_id,lane,side,amount FROM gomoku_bets WHERE game_id=$1 AND NOT settled ORDER BY account_id", g.ID)
	if err != nil {
		return err
	}
	bets := []gomokuBet{}
	for rows.Next() {
		var b gomokuBet
		if err = rows.Scan(&b.AccountID, &b.Lane, &b.Side, &b.Amount); err != nil {
			rows.Close()
			return err
		}
		bets = append(bets, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, b := range bets {
		payout := 0
		if winner == 0 {
			payout = b.Amount
		} else if b.Side == winner {
			payout = b.Amount * 2
		}
		if err = gomokuMoney(ctx, tx, g.ID, b.AccountID, b.Lane, "settle", payout); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE gomoku_bets SET settled=true,payout=$3 WHERE game_id=$1 AND account_id=$2", g.ID, b.AccountID, payout); err != nil {
			return err
		}
	}
	g.Status = "ended"
	g.Winner = winner
	g.Reason = reason
	g.Deadline = time.Time{}
	return gomokuSave(ctx, tx, g)
}
func gomokuAdvance(ctx context.Context, tx pgx.Tx, g *gomokuGame, now time.Time, restart bool) (bool, error) {
	if g == nil || g.Status == "ended" {
		return false, nil
	}
	if restart {
		return true, gomokuSettle(ctx, tx, g, 0, "系统维护，本局退款")
	}
	if now.Before(g.Deadline) {
		return false, nil
	}
	switch g.Status {
	case "waiting":
		return true, gomokuSettle(ctx, tx, g, 0, "等待超时，本局退款")
	case "betting":
		g.Status = "playing"
		g.Turn = 1
		g.Deadline = now.Add(120 * time.Second)
		return true, gomokuSave(ctx, tx, g)
	case "playing":
		return true, gomokuSettle(ctx, tx, g, 3-g.Turn, "落子超时")
	}
	return false, nil
}
func (a *app) tickGomoku(ctx context.Context, restart bool) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	g, err := gomokuLock(ctx, tx)
	if err != nil {
		return err
	}
	changed, err := gomokuAdvance(ctx, tx, g, time.Now().UTC(), restart)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err == nil && changed {
		a.gomoku.publish(gomokuEvent{Kind: "state", GameID: g.ID, Status: g.Status})
	}
	return err
}
func (a *app) runGomoku(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			work, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := a.tickGomoku(work, false)
			cancel()
			if err != nil && ctx.Err() == nil {
				log.Printf("gomoku tick: %v", err)
			}
		}
	}
}
func (a *app) gomokuAction(w http.ResponseWriter, r *http.Request) {
	var in gomokuInput
	if !decode(w, r, &in) {
		return
	}
	if len(in.RequestID) < 16 || len(in.RequestID) > 80 || strings.Trim(in.RequestID, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" {
		fail(w, 400, "请求编号不正确")
		return
	}
	owner := r.Context().Value(guestKey).(string)
	if !a.limits.allow("gomoku-action:"+owner, 120) {
		fail(w, 429, "慢一点，棋盘还在这里")
		return
	}
	ctx := r.Context()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	g, err := gomokuLock(ctx, tx)
	if err != nil {
		a.internal(w, err)
		return
	}
	raw, _ := json.Marshal(in)
	fingerprint := hash(string(raw))
	var old string
	err = tx.QueryRow(ctx, "SELECT fingerprint FROM gomoku_requests WHERE account_id=$1 AND request_id=$2", owner, in.RequestID).Scan(&old)
	if err == nil {
		if old != fingerprint {
			fail(w, 409, "请求已处理，请刷新棋盘")
			return
		}
		jsonOut(w, 200, map[string]bool{"ok": true})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		a.internal(w, err)
		return
	}
	changed, err := gomokuAdvance(ctx, tx, g, time.Now().UTC(), false)
	if err != nil {
		a.internal(w, err)
		return
	}
	// Commit time transitions even if the attempted move/bet is now too late.
	reject := func(code int, message string) {
		if changed {
			if err := tx.Commit(ctx); err != nil {
				a.internal(w, err)
				return
			}
			a.gomoku.publish(gomokuEvent{Kind: "state", GameID: g.ID, Status: g.Status})
		}
		fail(w, code, message)
	}
	if in.Action != "create" && (g == nil || g.ID != in.GameID) {
		reject(409, "这一局已变化，请刷新后操作")
		return
	}
	var player gomokuPlayer
	if in.Action == "create" || in.Action == "join" {
		player.ID = owner
		err = tx.QueryRow(ctx, "SELECT COALESCE(NULLIF(nickname,''),username),COALESCE((SELECT '/api/avatars/'||account_id||'?v='||version FROM account_avatars WHERE account_id=$1),'') FROM accounts WHERE id=$1", owner).Scan(&player.Name, &player.Avatar)
		if err != nil {
			a.internal(w, err)
			return
		}
	}
	switch in.Action {
	case "create":
		if g != nil && g.Status != "ended" {
			reject(409, "棋桌正在使用，先围观一下")
			return
		}
		if in.Amount != 0 && in.Amount != 10 && in.Amount != 50 && in.Amount != 100 {
			reject(400, "请选择0、10、50或100点")
			return
		}
		// An ended game ID is required, preventing a stale page replacing a newer game.
		if g != nil && g.ID != in.GameID {
			reject(409, "棋桌已更新，请刷新")
			return
		}
		g = &gomokuGame{ID: token(), Status: "waiting", Stake: in.Amount, Moves: []gomokuMove{}, Deadline: time.Now().UTC().Add(2 * time.Minute)}
		g.Players[0] = player
		_, err = tx.Exec(ctx, "INSERT INTO gomoku_games(id,data) VALUES($1,$2)", g.ID, g)
		if err == nil {
			_, err = tx.Exec(ctx, "UPDATE gomoku_table SET game_id=$1 WHERE id=1", g.ID)
		}
		if err == nil {
			err = gomokuMoney(ctx, tx, g.ID, owner, "player", in.RequestID, -g.Stake)
		}
		if err == nil {
			_, err = tx.Exec(ctx, "INSERT INTO gomoku_bets(game_id,account_id,lane,side,amount) VALUES($1,$2,'player',1,$3)", g.ID, owner, g.Stake)
		}
	case "join":
		if g.Status != "waiting" || g.side(owner) != 0 {
			reject(409, "当前不能加入对局")
			return
		}
		if in.Amount != g.Stake {
			reject(409, "本局押注已变化，请重新确认")
			return
		}
		err = gomokuMoney(ctx, tx, g.ID, owner, "player", in.RequestID, -g.Stake)
		if err == nil {
			_, err = tx.Exec(ctx, "INSERT INTO gomoku_bets(game_id,account_id,lane,side,amount) VALUES($1,$2,'player',2,$3)", g.ID, owner, g.Stake)
		}
		if err == nil {
			g.Players[1] = player
			if choice([]string{"black", "white"}) == "white" {
				g.Players[0], g.Players[1] = g.Players[1], g.Players[0]
				_, err = tx.Exec(ctx, "UPDATE gomoku_bets SET side=3-side WHERE game_id=$1", g.ID)
			}
			g.Status = "betting"
			g.Deadline = time.Now().UTC().Add(30 * time.Second)
		}
	case "bet":
		if g.Status != "betting" {
			reject(409, "已经封盘，不能改押注了")
			return
		}
		if g.side(owner) != 0 {
			reject(403, "棋手不能参与自己这局的场外竞猜")
			return
		}
		if in.Amount < 0 || in.Amount > 100 || (in.Side != 1 && in.Side != 2) {
			reject(400, "每人每局最多押100点，请选择黑方或白方")
			return
		}
		var previous, total int
		err = tx.QueryRow(ctx, "SELECT COALESCE(sum(amount) FILTER(WHERE account_id=$2),0),COALESCE(sum(amount),0) FROM gomoku_bets WHERE game_id=$1 AND lane='audience'", g.ID, owner).Scan(&previous, &total)
		if err != nil {
			a.internal(w, err)
			return
		}
		if total-previous+in.Amount > 2000 {
			reject(409, "本局场外总押注上限2000点，先看棋吧")
			return
		}
		err = gomokuMoney(ctx, tx, g.ID, owner, "audience", in.RequestID, previous-in.Amount)
		if err == nil {
			_, err = tx.Exec(ctx, "INSERT INTO gomoku_bets(game_id,account_id,lane,side,amount) VALUES($1,$2,'audience',$3,$4) ON CONFLICT(game_id,account_id) DO UPDATE SET side=EXCLUDED.side,amount=EXCLUDED.amount", g.ID, owner, in.Side, in.Amount)
		}
	case "move":
		side := g.side(owner)
		if g.Status != "playing" || side == 0 || g.Turn != side {
			reject(409, "还没轮到你落子")
			return
		}
		if in.Ply != len(g.Moves) {
			reject(409, "棋盘已更新，请重新选择落点")
			return
		}
		if in.X < 0 || in.X >= 15 || in.Y < 0 || in.Y >= 15 || g.Board[in.Y*15+in.X] != 0 {
			reject(400, "这里不能落子")
			return
		}
		g.Board[in.Y*15+in.X] = side
		g.Moves = append(g.Moves, gomokuMove{in.X, in.Y, side})
		if gomokuWon(g.Board, in.X, in.Y, side) {
			err = gomokuSettle(ctx, tx, g, side, "五子连珠")
		} else if len(g.Moves) == 225 {
			err = gomokuSettle(ctx, tx, g, 0, "棋盘已满，平局退款")
		} else {
			g.Turn = 3 - side
			g.Deadline = time.Now().UTC().Add(120 * time.Second)
		}
	case "leave":
		if g.side(owner) == 0 || (g.Status != "waiting" && g.Status != "betting") {
			reject(409, "开局后请使用认输")
			return
		}
		err = gomokuSettle(ctx, tx, g, 0, "棋手离座，本局退款")
	case "resign":
		side := g.side(owner)
		if g.Status != "playing" || side == 0 {
			reject(409, "当前不能认输")
			return
		}
		err = gomokuSettle(ctx, tx, g, 3-side, "对手认输")
	case "abort":
		if owner != a.adminAccountID {
			reject(403, "仅店长可以中止对局")
			return
		}
		if g.Status == "ended" {
			reject(409, "本局已结束")
			return
		}
		err = gomokuSettle(ctx, tx, g, 0, "店长中止，本局退款")
	default:
		reject(400, "操作不正确")
		return
	}
	// Insufficient funds or persistence errors roll back every balance/ledger change.
	if err != nil {
		if errors.Is(err, errGomokuBalance) {
			fail(w, 409, err.Error())
		} else {
			a.internal(w, err)
		}
		return
	}
	if err = gomokuSave(ctx, tx, g); err == nil {
		_, err = tx.Exec(ctx, "INSERT INTO gomoku_requests(account_id,request_id,fingerprint) VALUES($1,$2,$3)", owner, in.RequestID, fingerprint)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	a.gomoku.publish(gomokuEvent{Kind: "state", GameID: g.ID, Status: g.Status})
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (a *app) gomokuState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	owner := r.Context().Value(guestKey).(string)
	tx, err := a.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var g *gomokuGame
	err = tx.QueryRow(ctx, "SELECT g.data FROM gomoku_table t LEFT JOIN gomoku_games g ON g.id=t.game_id WHERE t.id=1").Scan(&g)
	if err != nil {
		a.internal(w, err)
		return
	}
	var balance int
	if err = tx.QueryRow(ctx, "SELECT balance FROM guests WHERE id=$1", owner).Scan(&balance); err != nil {
		a.internal(w, err)
		return
	}
	var mine *gomokuBet
	totals := [2]int{}
	if g != nil {
		rows, e := tx.Query(ctx, "SELECT account_id,lane,side,amount,payout,settled FROM gomoku_bets WHERE game_id=$1", g.ID)
		if e != nil {
			a.internal(w, e)
			return
		}
		for rows.Next() {
			var b gomokuBet
			if e = rows.Scan(&b.AccountID, &b.Lane, &b.Side, &b.Amount, &b.Payout, &b.Settled); e != nil {
				rows.Close()
				a.internal(w, e)
				return
			}
			if b.AccountID == owner {
				copy := b
				mine = &copy
			}
			if b.Lane == "audience" {
				totals[b.Side-1] += b.Amount
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			a.internal(w, e)
			return
		}
	}
	jsonOut(w, 200, map[string]any{"game": g, "mine": mine, "totals": totals, "balance": balance, "online": a.gomoku.online(), "server_time": time.Now().UTC()})
}
func (a *app) gomokuHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT g.data,COALESCE((SELECT jsonb_agg(jsonb_build_object('username',a.username,'nickname',a.nickname,'lane',b.lane,'side',b.side,'amount',b.amount,'payout',b.payout,'settled',b.settled)) FROM gomoku_bets b JOIN accounts a ON a.id=b.account_id WHERE b.game_id=g.id),'[]'::jsonb),COALESCE((SELECT sum(amount-payout) FROM gomoku_bets b WHERE b.game_id=g.id AND b.lane='audience' AND b.settled),0) FROM gomoku_games g ORDER BY g.created_at DESC LIMIT 20`)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var g gomokuGame
		var bets json.RawMessage
		var net int
		if err = rows.Scan(&g, &bets, &net); err != nil {
			a.internal(w, err)
			return
		}
		items = append(items, map[string]any{"game": g, "bets": bets, "platform_net": net})
	}
	if err = rows.Err(); err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"items": items})
}
