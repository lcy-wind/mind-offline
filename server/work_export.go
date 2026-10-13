package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func workFilters(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "active" && !validWorkStatus(status) {
		fail(w, 400, "工作状态不正确")
		return "", "", false
	}
	q, ok := searchQuery(w, r)
	return status, q, ok
}
func (a *app) filteredWork(ctx context.Context, owner, status, q string) ([]workTask, error) {
	rows, err := a.db.Query(ctx, "SELECT "+workColumns+" FROM work_tasks WHERE account_id=$1 AND ($2='all' OR ($2='active' AND status<>'done') OR status=$2) AND ($3='' OR strpos(lower(title),lower($3))>0) ORDER BY (status='done'),urgency DESC,planned_at ASC NULLS LAST,created_at DESC,id", owner, status, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []workTask{}
	for rows.Next() {
		task, err := scanWork(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}
func (a *app) workExport(w http.ResponseWriter, r *http.Request) {
	a.exportWorkFor(w, r, r.Context().Value(guestKey).(string))
}
func (a *app) adminWorkExport(w http.ResponseWriter, r *http.Request) {
	a.exportWorkFor(w, r, r.PathValue("id"))
}
func (a *app) exportWorkFor(w http.ResponseWriter, r *http.Request, owner string) {
	status, q, ok := workFilters(w, r)
	if !ok {
		return
	}
	var username string
	if err := a.db.QueryRow(r.Context(), "SELECT username FROM accounts WHERE id=$1", owner).Scan(&username); errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "顾客不存在")
		return
	} else if err != nil {
		a.internal(w, err)
		return
	}
	tasks, err := a.filteredWork(r.Context(), owner, status, q)
	if err != nil {
		a.internal(w, err)
		return
	}
	data, err := workWorkbook(username, tasks)
	if err != nil {
		a.internal(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="work-tasks-`+time.Now().UTC().Format("20060102-150405")+`.xlsx"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(200)
	_, _ = w.Write(data)
}

// A single-sheet XLSX report with inline strings, avoiding formula execution for
// user-supplied titles/emails. No spreadsheet libraries or external files needed.
func workWorkbook(username string, tasks []workTask) ([]byte, error) {
	headers := []string{"序号", "顾客", "工作内容", "计划时间（北京时间）", "紧急程度", "进度", "收件邮箱", "提醒时间（北京时间）", "提醒状态", "发送时间（北京时间）", "提醒说明", "创建时间（北京时间）", "更新时间（北京时间）"}
	widths := []int{7, 18, 44, 24, 12, 12, 30, 24, 16, 24, 40, 24, 24}
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews><sheetFormatPr defaultRowHeight="20"/><cols>`)
	for i, width := range widths {
		fmt.Fprintf(&sheet, `<col min="%d" max="%d" width="%d" customWidth="1"/>`, i+1, i+1, width)
	}
	sheet.WriteString(`</cols><sheetData><row r="1" ht="36" customHeight="1">`)
	cell := func(col, row, style int, text string) {
		fmt.Fprintf(&sheet, `<c r="%s%d" s="%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, excelColumn(col), row, style, spreadsheetText(text))
	}
	dateCell := func(col, row int, value *time.Time) {
		if value == nil {
			cell(col, row, 0, "")
			return
		}
		serial := (float64(value.Unix())+8*3600)/86400 + 25569
		fmt.Fprintf(&sheet, `<c r="%s%d" s="2"><v>%s</v></c>`, excelColumn(col), row, strconv.FormatFloat(serial, 'f', 10, 64))
	}
	for i, h := range headers {
		cell(i, 1, 1, h)
	}
	sheet.WriteString(`</row>`)
	statusNames := map[string]string{"todo": "待开始", "doing": "进行中", "done": "已完成"}
	reminderNames := map[string]string{"off": "未开启", "pending": "等待发送", "sending": "正在发送", "sent": "已发送", "failed": "发送失败", "cancelled": "已取消", "uncertain": "结果待确认"}
	urgencyNames := map[int]string{1: "不急", 2: "普通", 3: "紧急"}
	for i, t := range tasks {
		row := i + 2
		height := math.Max(32, math.Ceil(float64(utf8.RuneCountInString(t.Title))/20)*16+8)
		height = math.Max(height, float64(strings.Count(t.Title, "\n")+1)*16+8)
		height = math.Min(409, math.Max(height, math.Ceil(float64(len(t.ReminderEmail))/30)*16+8))
		fmt.Fprintf(&sheet, `<row r="%d" ht="%.0f" customHeight="1"><c r="A%d"><v>%d</v></c>`, row, height, row, i+1)
		cell(1, row, 0, username)
		cell(2, row, 0, t.Title)
		dateCell(3, row, t.PlannedAt)
		cell(4, row, 0, urgencyNames[t.Urgency])
		cell(5, row, 0, statusNames[t.Status])
		cell(6, row, 0, t.ReminderEmail)
		dateCell(7, row, t.ReminderAt)
		cell(8, row, 0, reminderNames[t.ReminderState])
		dateCell(9, row, t.ReminderSentAt)
		cell(10, row, 0, t.ReminderError)
		dateCell(11, row, &t.CreatedAt)
		dateCell(12, row, &t.UpdatedAt)
		sheet.WriteString(`</row>`)
	}
	fmt.Fprintf(&sheet, `</sheetData><autoFilter ref="A1:M%d"/></worksheet>`, len(tasks)+1)
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="牛马工作记录" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`},
		{"xl/styles.xml", `<?xml version="1.0"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><numFmts count="1"><numFmt numFmtId="164" formatCode="yyyy-mm-dd hh:mm"/></numFmts><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><color rgb="FFFFFFFF"/><name val="Calibri"/></font></fonts><fills count="3"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF82704F"/><bgColor indexed="64"/></patternFill></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="3"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0" applyAlignment="1"><alignment vertical="top" wrapText="1"/></xf><xf numFmtId="0" fontId="1" fillId="2" borderId="0" xfId="0" applyFont="1" applyFill="1" applyAlignment="1"><alignment vertical="center" wrapText="1"/></xf><xf numFmtId="164" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyAlignment="1"><alignment vertical="top"/></xf></cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>`},
		{"xl/worksheets/sheet1.xml", sheet.String()},
	}
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	for _, p := range parts {
		file, err := archive.Create(p.name)
		if err != nil {
			return nil, err
		}
		if _, err = file.Write([]byte(p.body)); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
func excelColumn(index int) string {
	out := ""
	for index >= 0 {
		out = string(rune('A'+index%26)) + out
		index = index/26 - 1
	}
	return out
}
func spreadsheetText(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' || r == 0xfffe || r == 0xffff {
			return '\ufffd'
		}
		return r
	}, s)
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
