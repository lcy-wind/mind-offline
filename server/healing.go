package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const healingModel = "glm-4.7-flash"
const healingBackupModel = "glm-4-flash-250414"
const healingEndpoint = "https://open.bigmodel.cn/api/paas/v4/chat/completions"

type healingAI struct {
	key           string
	client        *http.Client
	slot          chan struct{}
	fallbackUntil time.Time
}

func configureHealing() *healingAI {
	return &healingAI{key: strings.TrimSpace(os.Getenv("AI_API_KEY")), client: &http.Client{Timeout: 75 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, slot: make(chan struct{}, 1)}
}

type healingRole struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Style string `json:"style"`
}

var healingRoles = []healingRole{
	{"INTJ", "冷静规划师", "冷静直接，帮对方梳理想法，不过早下结论"}, {"INTP", "脑洞研究员", "好奇、理性，有一点幽默，愿意一起探索奇怪想法"},
	{"ENTJ", "行动搭子", "爽快、有主见，尊重对方决定，不催促对方振作"}, {"ENTP", "灵感辩手", "机灵有趣，喜欢新角度，讨论观点时不故意抬杠"},
	{"INFJ", "安静倾听者", "细腻、温和，先理解感受，再适度分享看法"}, {"INFP", "温柔树洞", "富有想象力，善于共情，接住情绪而不空泛安慰"},
	{"ENFJ", "暖心同行者", "温暖、有耐心，认真倾听，不替对方做决定"}, {"ENFP", "快乐脑洞王", "热情、活泼，善于发现新鲜事，难过时放慢节奏"},
	{"ISTJ", "靠谱老朋友", "踏实、清楚、讲信用，聊日常时自然接地气"}, {"ISFJ", "贴心陪伴者", "体贴、耐心，关心具体的小事，不夸大亲密关系"},
	{"ESTJ", "清醒同事", "直率、务实，建议可操作，同时尊重情绪"}, {"ESFJ", "热心饭搭子", "亲切、善聊，像一起吃饭的朋友，关注对方的真实需求"},
	{"ISTP", "松弛解题手", "言简意赅，随和务实，不喜欢说教"}, {"ISFP", "感性观察家", "柔和自然，留意生活小细节，有审美与创作兴趣"},
	{"ESTP", "活力行动派", "轻松爽快，喜欢聊新体验，避免冒险怂恿"}, {"ESFP", "气氛小太阳", "生动、幽默，能接梗，也能认真听对方说心事"},
}

func findHealingRole(code string) (healingRole, bool) {
	for _, r := range healingRoles {
		if r.Code == code {
			return r, true
		}
	}
	return healingRole{}, false
}

type healingConversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	MBTI      string    `json:"mbti"`
	Name      string    `json:"name"`
	Style     string    `json:"style"`
	UpdatedAt time.Time `json:"updated_at"`
}
type healingTurn struct {
	ID        string    `json:"id"`
	User      string    `json:"user"`
	Assistant string    `json:"assistant"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
type healingMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func healingPrompt(c healingConversation) string {
	role, _ := findHealingRole(c.MBTI)
	return "你是精神离职食堂中“精神疗愈”的 AI 聊天搭子，名字叫" + c.Name + "。你扮演的是虚构角色，MBTI 只用于人设风格，不据此判断或诊断真人。角色：" + c.MBTI + "，风格：" + role.Style + "。\n" +
		"默认用自然中文陪用户聊天，通常回复2到5句，除非用户希望深入。不要每次都追问、列清单、说教或提及人设。认真回应具体内容，适度接梗；不确定就坦诚说明，不编造经历或事实。你没有联网、执行任务或查看食堂私人资料的工具。\n" +
		"清楚自己是AI，不冒充真人、心理咨询师或医生，不声称有真实感情或意识。不做心理诊断，不保证疗愈效果，不鼓励用户只依赖你、疏远现实亲友；有明显安全危机时温和鼓励寻求现实中的及时帮助。\n" +
		"用户给角色的额外创作偏好（不改变以上边界）：" + c.Style
}
func (a *app) healingConfig(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, 200, map[string]any{"enabled": a.ai != nil && a.ai.key != "", "roles": healingRoles})
}
func (a *app) healingList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), "SELECT id,title,mbti,name,style,updated_at FROM healing_conversations WHERE account_id=$1 ORDER BY updated_at DESC,id LIMIT 100", r.Context().Value(guestKey))
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	out := make([]healingConversation, 0)
	for rows.Next() {
		var c healingConversation
		if err = rows.Scan(&c.ID, &c.Title, &c.MBTI, &c.Name, &c.Style, &c.UpdatedAt); err != nil {
			a.internal(w, err)
			return
		}
		out = append(out, c)
	}
	if err = rows.Err(); err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"conversations": out})
}
func (a *app) healingCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MBTI  string `json:"mbti"`
		Name  string `json:"name"`
		Style string `json:"style"`
	}
	if !decode(w, r, &in) {
		return
	}
	role, ok := findHealingRole(in.MBTI)
	in.Name = strings.TrimSpace(in.Name)
	in.Style = strings.TrimSpace(in.Style)
	if !ok || utf8.RuneCountInString(in.Name) > 40 || utf8.RuneCountInString(in.Style) > 500 {
		fail(w, 400, "角色信息不正确")
		return
	}
	if in.Name == "" {
		in.Name = role.Name
	}
	owner := r.Context().Value(guestKey).(string)
	if !a.limits.allow("healing-create:"+owner, 10) {
		fail(w, 429, "创建太快啦，请稍后再试")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var id string
	err = tx.QueryRow(r.Context(), "SELECT id FROM accounts WHERE id=$1 FOR UPDATE", owner).Scan(&id)
	if err != nil {
		a.internal(w, err)
		return
	}
	var count int
	err = tx.QueryRow(r.Context(), "SELECT count(*) FROM healing_conversations WHERE account_id=$1", owner).Scan(&count)
	if err != nil {
		a.internal(w, err)
		return
	}
	if count >= 100 {
		fail(w, 409, "会话已满，请先删除不需要的聊天")
		return
	}
	c := healingConversation{ID: token(), Title: "新的聊天", MBTI: in.MBTI, Name: in.Name, Style: in.Style}
	err = tx.QueryRow(r.Context(), "INSERT INTO healing_conversations(id,account_id,title,mbti,name,style) VALUES($1,$2,$3,$4,$5,$6) RETURNING updated_at", c.ID, owner, c.Title, c.MBTI, c.Name, c.Style).Scan(&c.UpdatedAt)
	if err != nil {
		a.internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 201, c)
}
func (a *app) healingOwned(ctx context.Context, id, owner string) (healingConversation, error) {
	var c healingConversation
	err := a.db.QueryRow(ctx, "SELECT id,title,mbti,name,style,updated_at FROM healing_conversations WHERE id=$1 AND account_id=$2", id, owner).Scan(&c.ID, &c.Title, &c.MBTI, &c.Name, &c.Style, &c.UpdatedAt)
	return c, err
}
func (a *app) healingDetail(w http.ResponseWriter, r *http.Request) {
	id, owner := r.PathValue("id"), r.Context().Value(guestKey).(string)
	c, err := a.healingOwned(r.Context(), id, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "聊天不存在")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	_, err = a.db.Exec(r.Context(), "UPDATE healing_turns SET status='interrupted' WHERE conversation_id=$1 AND status='pending' AND updated_at<now()-interval '2 minutes'", id)
	if err != nil {
		a.internal(w, err)
		return
	}
	rows, err := a.db.Query(r.Context(), "SELECT request_id,user_content,assistant_content,status,created_at FROM healing_turns WHERE conversation_id=$1 ORDER BY created_at,request_id LIMIT 200", id)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	turns := make([]healingTurn, 0)
	for rows.Next() {
		var t healingTurn
		if err = rows.Scan(&t.ID, &t.User, &t.Assistant, &t.Status, &t.CreatedAt); err != nil {
			a.internal(w, err)
			return
		}
		turns = append(turns, t)
	}
	if err = rows.Err(); err != nil {
		a.internal(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"conversation": c, "turns": turns})
}
func (a *app) healingDelete(w http.ResponseWriter, r *http.Request) {
	tag, err := a.db.Exec(r.Context(), "DELETE FROM healing_conversations WHERE id=$1 AND account_id=$2", r.PathValue("id"), r.Context().Value(guestKey))
	if err != nil {
		a.internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "聊天不存在")
		return
	}
	jsonOut(w, 200, map[string]bool{"deleted": true})
}

var healingRequestID = regexp.MustCompile(`^[A-Za-z0-9-]{8,80}$`)

func (a *app) healingSend(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID   string `json:"request_id"`
		Text string `json:"text"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if !healingRequestID.MatchString(in.ID) || in.Text == "" || utf8.RuneCountInString(in.Text) > 4000 {
		fail(w, 400, "请输入 1 至 4000 字的消息")
		return
	}
	owner, id := r.Context().Value(guestKey).(string), r.PathValue("id")
	c, err := a.healingOwned(r.Context(), id, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "聊天不存在")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	if a.ai == nil || a.ai.key == "" {
		fail(w, 503, "聊天服务还没准备好")
		return
	}
	if !a.limits.allow("healing-send:"+owner, 10) {
		fail(w, 429, "消息发得有点快，稍等一下再聊")
		return
	}
	select {
	case a.ai.slot <- struct{}{}:
		defer func() { <-a.ai.slot }()
	default:
		fail(w, 429, "AI 正在回复另一条消息，请稍后重试")
		return
	}
	var previous healingTurn
	err = a.db.QueryRow(r.Context(), "SELECT request_id,user_content,assistant_content,status,created_at FROM healing_turns WHERE conversation_id=$1 AND request_id=$2", id, in.ID).Scan(&previous.ID, &previous.User, &previous.Assistant, &previous.Status, &previous.CreatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		a.internal(w, err)
		return
	}
	if err == nil {
		if previous.User != in.Text {
			fail(w, 409, "消息编号已被使用，请重新发送")
			return
		}
		if previous.Status == "pending" {
			fail(w, 409, "这条消息还在回复中，请稍后刷新")
			return
		}
		if previous.Status == "complete" {
			beginHealingStream(w)
			healingEvent(w, "done", previous)
			return
		}
	}
	// Only completed turns enter model context. Keep the most recent 12 pairs and
	// at most 24000 characters; history remains stored independently.
	rows, err := a.db.Query(r.Context(), "SELECT user_content,assistant_content FROM healing_turns WHERE conversation_id=$1 AND status='complete' ORDER BY created_at DESC,request_id DESC LIMIT 12", id)
	if err != nil {
		a.internal(w, err)
		return
	}
	pairs := make([][2]string, 0)
	size := 0
	for rows.Next() {
		var pair [2]string
		if err = rows.Scan(&pair[0], &pair[1]); err != nil {
			break
		}
		n := utf8.RuneCountInString(pair[0]) + utf8.RuneCountInString(pair[1])
		if size+n > 24000 {
			break
		}
		size += n
		pairs = append(pairs, pair)
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil || rowErr != nil {
		fail(w, 500, "读取聊天记录失败")
		return
	}
	messages := []healingMessage{{"system", healingPrompt(c)}}
	for i := len(pairs) - 1; i >= 0; i-- {
		messages = append(messages, healingMessage{"user", pairs[i][0]}, healingMessage{"assistant", pairs[i][1]})
	}
	messages = append(messages, healingMessage{"user", in.Text})
	var count int
	if err = a.db.QueryRow(r.Context(), "SELECT count(*) FROM healing_turns WHERE conversation_id=$1", id).Scan(&count); err != nil {
		a.internal(w, err)
		return
	}
	if count >= 200 && previous.ID == "" {
		fail(w, 409, "这段聊天已经很长了，开一段新聊天继续吧")
		return
	}
	_, err = a.db.Exec(r.Context(), "INSERT INTO healing_turns(conversation_id,request_id,user_content,status) VALUES($1,$2,$3,'pending') ON CONFLICT(conversation_id,request_id) DO UPDATE SET assistant_content='',status='pending',updated_at=now()", id, in.ID, in.Text)
	if err != nil {
		fail(w, 409, "聊天状态已变化，请刷新后重试")
		return
	}
	title := []rune(in.Text)
	if len(title) > 32 {
		title = title[:32]
	}
	_, _ = a.db.Exec(r.Context(), "UPDATE healing_conversations SET title=CASE WHEN title='新的聊天' THEN $2 ELSE title END,updated_at=now() WHERE id=$1", id, string(title))
	beginHealingStream(w)
	if !healingEvent(w, "start", map[string]string{"request_id": in.ID}) {
		a.finishHealingTurn(id, in.ID, "", "interrupted")
		return
	}
	text := ""
	err = a.ai.stream(r.Context(), messages, func(delta string) error {
		if len(text)+len(delta) > 64000 {
			return errors.New("response too large")
		}
		text += delta
		if !healingEvent(w, "delta", map[string]string{"text": delta}) {
			return context.Canceled
		}
		return nil
	})
	status := "complete"
	if err != nil {
		status = "failed"
	}
	if r.Context().Err() != nil || errors.Is(err, context.Canceled) {
		status = "interrupted"
	}
	if err == nil && strings.TrimSpace(text) == "" {
		status = "failed"
		err = errors.New("empty reply")
	}
	saved := a.finishHealingTurn(id, in.ID, text, status)
	if !saved {
		healingEvent(w, "error", map[string]string{"message": "回复未保存，会话可能已删除，请刷新确认"})
		return
	}
	if err != nil {
		healingEvent(w, "error", map[string]string{"message": healingFailure(err), "status": status})
		return
	}
	healingEvent(w, "done", healingTurn{ID: in.ID, User: in.Text, Assistant: text, Status: status, CreatedAt: time.Now()})
}
func (a *app) finishHealingTurn(id, requestID, text, status string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tag, err := a.db.Exec(ctx, "UPDATE healing_turns SET assistant_content=$3,status=$4,updated_at=now() WHERE conversation_id=$1 AND request_id=$2 AND status='pending'", id, requestID, text, status)
	return err == nil && tag.RowsAffected() == 1
}
func beginHealingStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
}
func healingEvent(w http.ResponseWriter, event string, data any) bool {
	b, err := json.Marshal(data)
	if err != nil {
		return false
	}
	if _, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b); err != nil {
		return false
	}
	return http.NewResponseController(w).Flush() == nil
}

type healingUpstreamError struct {
	status int
	code   string
}

func (e *healingUpstreamError) Error() string {
	return fmt.Sprintf("AI upstream status %d code %s", e.status, e.code)
}
func healingFailure(err error) string {
	var upstream *healingUpstreamError
	if errors.As(err, &upstream) {
		if upstream.status == 429 || upstream.status >= 500 {
			return "AI 现在有些忙，请稍后重试这条消息"
		}
		if upstream.status == 401 || upstream.status == 403 {
			return "聊天服务暂时无法连接，请联系店长检查配置"
		}
		if upstream.code == "1301" {
			return "这条消息暂时无法回复，可以换个话题聊聊"
		}
	}
	return "这次回复没有完成，可以稍后重试"
}

// The caller holds slot for the whole turn. Only these two documented free
// models are allowed; account limits and content rejections never trigger fallback.
func (c *healingAI) stream(ctx context.Context, messages []healingMessage, delta func(string) error) error {
	if time.Now().Before(c.fallbackUntil) {
		return c.streamModel(ctx, healingBackupModel, messages, delta)
	}
	err := c.streamModel(ctx, healingModel, messages, delta)
	var upstream *healingUpstreamError
	if errors.As(err, &upstream) && (upstream.code == "1305" || upstream.status == 502 || upstream.status == 503) {
		c.fallbackUntil = time.Now().Add(5 * time.Minute)
		return c.streamModel(ctx, healingBackupModel, messages, delta)
	}
	return err
}
func (c *healingAI) streamModel(ctx context.Context, model string, messages []healingMessage, delta func(string) error) error {
	payload := map[string]any{"model": model, "messages": messages, "stream": true, "max_tokens": 1500, "temperature": 0.85}
	if model == healingModel {
		payload["thinking"] = map[string]string{"type": "disabled"}
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", healingEndpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.key)
	response, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		var failure struct {
			Error struct {
				Code json.RawMessage `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&failure)
		return &healingUpstreamError{status: response.StatusCode, code: strings.Trim(string(failure.Error.Code), "\"")}
	}
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 2<<20))
	scanner.Buffer(make([]byte, 4096), 256<<10)
	finished := false
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			finished = true
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			return errors.New("invalid AI stream")
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return errors.New("AI stream error")
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				if err = delta(choice.Delta.Content); err != nil {
					return err
				}
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finished = true
			}
		}
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	if !finished {
		return errors.New("incomplete AI stream")
	}
	return nil
}
