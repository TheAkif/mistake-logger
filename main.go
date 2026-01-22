package main

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"encoding/csv"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Mistake struct {
	ID              int64
	Topic           string
	Date            string
	Problem         string
	Missed          string
	FixRule         string
	PatternRemember string
	CreatedAt       string
}

type IndexData struct {
	Today    string
	QueryQ   string
	Topic    string
	From     string
	To       string
	Mistakes []Mistake
	Count    int
}

type EditData struct {
	Mistake Mistake
}

type RulesData struct {
	QueryQ string
	Topic  string
	From   string
	To     string
	Items  []Mistake
	Count  int
}

var (
	db   *sql.DB
	tmpl *template.Template
)

func main() {
	var err error

	db, err = sql.Open("sqlite", "mistakes.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := initSchema(db); err != nil {
		log.Fatal(err)
	}

	tmpl, err = template.ParseGlob(filepath.Join("templates", "*.html"))
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/add", handleAdd)
	mux.HandleFunc("/edit", handleEdit)
	mux.HandleFunc("/delete", handleDelete)
	mux.HandleFunc("/export.csv", handleExportCSV)
	mux.HandleFunc("/rules", handleRules)

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	addr := "127.0.0.1:8080"
	log.Printf("Listening on http://%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, withBasicSecurityHeaders(mux)))
}

func initSchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS mistakes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  topic TEXT NOT NULL,
  date TEXT NOT NULL, -- YYYY-MM-DD
  problem_statement TEXT NOT NULL,
  what_i_missed TEXT NOT NULL,
  fix_rule TEXT NOT NULL,
  pattern_to_remember TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_mistakes_date ON mistakes(date DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_mistakes_topic ON mistakes(topic);
`
	_, err := db.Exec(schema)
	return err
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	topic := strings.TrimSpace(r.URL.Query().Get("topic"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))

	if from != "" {
		if _, err := time.Parse("2006-01-02", from); err != nil {
			http.Error(w, "Invalid 'from' date. Use YYYY-MM-DD.", http.StatusBadRequest)
			return
		}
	}
	if to != "" {
		if _, err := time.Parse("2006-01-02", to); err != nil {
			http.Error(w, "Invalid 'to' date. Use YYYY-MM-DD.", http.StatusBadRequest)
			return
		}
	}

	mistakes, err := searchMistakes(db, q, topic, from, to, 200)
	if err != nil {
		http.Error(w, "Failed to load mistakes", http.StatusInternalServerError)
		log.Println("searchMistakes:", err)
		return
	}

	data := IndexData{
		Today:    time.Now().Format("2006-01-02"),
		QueryQ:   q,
		Topic:    topic,
		From:     from,
		To:       to,
		Mistakes: mistakes,
		Count:    len(mistakes),
	}

	if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Println("template execute index:", err)
	}
}

func handleAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad form", http.StatusBadRequest)
		return
	}

	m, err := mistakeFromForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m.CreatedAt = time.Now().Format(time.RFC3339)
	if err := insertMistake(db, m); err != nil {
		http.Error(w, "Failed to save mistake", http.StatusInternalServerError)
		log.Println("insertMistake:", err)
		return
	}

	redirectBack(w, r)
}

func handleEdit(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		id, ok := parseIDParam(r, "id")
		if !ok {
			http.Error(w, "Missing/invalid id", http.StatusBadRequest)
			return
		}

		m, err := getMistakeByID(db, id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "Failed to load mistake", http.StatusInternalServerError)
			log.Println("getMistakeByID:", err)
			return
		}

		if err := tmpl.ExecuteTemplate(w, "edit.html", EditData{Mistake: m}); err != nil {
			log.Println("template execute edit:", err)
		}

	case http.MethodPost:
		id, ok := parseIDParam(r, "id")
		if !ok {
			http.Error(w, "Missing/invalid id", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad form", http.StatusBadRequest)
			return
		}

		m, err := mistakeFromForm(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		m.ID = id

		if err := updateMistake(db, m); err != nil {
			http.Error(w, "Failed to update mistake", http.StatusInternalServerError)
			log.Println("updateMistake:", err)
			return
		}

		redirectBack(w, r)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, ok := parseIDParam(r, "id")
	if !ok {
		http.Error(w, "Missing/invalid id", http.StatusBadRequest)
		return
	}

	if err := deleteMistake(db, id); err != nil {
		http.Error(w, "Failed to delete mistake", http.StatusInternalServerError)
		log.Println("deleteMistake:", err)
		return
	}

	redirectBack(w, r)
}

func handleExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	topic := strings.TrimSpace(r.URL.Query().Get("topic"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))

	if from != "" {
		if _, err := time.Parse("2006-01-02", from); err != nil {
			http.Error(w, "Invalid 'from' date. Use YYYY-MM-DD.", http.StatusBadRequest)
			return
		}
	}
	if to != "" {
		if _, err := time.Parse("2006-01-02", to); err != nil {
			http.Error(w, "Invalid 'to' date. Use YYYY-MM-DD.", http.StatusBadRequest)
			return
		}
	}

	items, err := searchMistakes(db, q, topic, from, to, 10000)
	if err != nil {
		http.Error(w, "Failed to export", http.StatusInternalServerError)
		log.Println("export searchMistakes:", err)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="mistakes.csv"`)

	cw := csv.NewWriter(w)
	defer cw.Flush()

	_ = cw.Write([]string{"id", "date", "topic", "problem_statement", "what_i_missed", "fix_rule", "pattern_to_remember", "created_at"})
	for _, m := range items {
		_ = cw.Write([]string{
			strconv.FormatInt(m.ID, 10),
			m.Date,
			m.Topic,
			m.Problem,
			m.Missed,
			m.FixRule,
			m.PatternRemember,
			m.CreatedAt,
		})
	}
}

func handleRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	topic := strings.TrimSpace(r.URL.Query().Get("topic"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))

	if from != "" {
		if _, err := time.Parse("2006-01-02", from); err != nil {
			http.Error(w, "Invalid 'from' date. Use YYYY-MM-DD.", http.StatusBadRequest)
			return
		}
	}
	if to != "" {
		if _, err := time.Parse("2006-01-02", to); err != nil {
			http.Error(w, "Invalid 'to' date. Use YYYY-MM-DD.", http.StatusBadRequest)
			return
		}
	}

	items, err := searchMistakes(db, q, topic, from, to, 500)
	if err != nil {
		http.Error(w, "Failed to load rules", http.StatusInternalServerError)
		log.Println("rules searchMistakes:", err)
		return
	}

	data := RulesData{
		QueryQ: q,
		Topic:  topic,
		From:   from,
		To:     to,
		Items:  items,
		Count:  len(items),
	}

	if err := tmpl.ExecuteTemplate(w, "rules.html", data); err != nil {
		log.Println("template execute rules:", err)
	}
}

// DB OEPRATIONS

func searchMistakes(db *sql.DB, q, topic, from, to string, limit int) ([]Mistake, error) {
	var where []string
	var args []any

	if topic != "" {
		where = append(where, "topic = ?")
		args = append(args, topic)
	}
	if from != "" {
		where = append(where, "date >= ?")
		args = append(args, from)
	}
	if to != "" {
		where = append(where, "date <= ?")
		args = append(args, to)
	}
	if q != "" {
		like := "%" + q + "%"
		where = append(where, `(topic LIKE ? OR problem_statement LIKE ? OR what_i_missed LIKE ? OR fix_rule LIKE ? OR pattern_to_remember LIKE ?)`)
		args = append(args, like, like, like, like, like)
	}

	query := `
SELECT id, topic, date, problem_statement, what_i_missed, fix_rule, pattern_to_remember, created_at
FROM mistakes
`
	if len(where) > 0 {
		query += "WHERE " + strings.Join(where, " AND ") + "\n"
	}
	query += "ORDER BY date DESC, id DESC\nLIMIT ?;"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Mistake
	for rows.Next() {
		var m Mistake
		if err := rows.Scan(&m.ID, &m.Topic, &m.Date, &m.Problem, &m.Missed, &m.FixRule, &m.PatternRemember, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func insertMistake(db *sql.DB, m Mistake) error {
	_, err := db.Exec(`
INSERT INTO mistakes (topic, date, problem_statement, what_i_missed, fix_rule, pattern_to_remember, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);
`, m.Topic, m.Date, m.Problem, m.Missed, m.FixRule, m.PatternRemember, m.CreatedAt)
	return err
}

func getMistakeByID(db *sql.DB, id int64) (Mistake, error) {
	var m Mistake
	err := db.QueryRow(`
SELECT id, topic, date, problem_statement, what_i_missed, fix_rule, pattern_to_remember, created_at
FROM mistakes
WHERE id = ?;
`, id).Scan(&m.ID, &m.Topic, &m.Date, &m.Problem, &m.Missed, &m.FixRule, &m.PatternRemember, &m.CreatedAt)
	return m, err
}

func updateMistake(db *sql.DB, m Mistake) error {
	_, err := db.Exec(`
UPDATE mistakes
SET topic = ?, date = ?, problem_statement = ?, what_i_missed = ?, fix_rule = ?, pattern_to_remember = ?
WHERE id = ?;
`, m.Topic, m.Date, m.Problem, m.Missed, m.FixRule, m.PatternRemember, m.ID)
	return err
}

func deleteMistake(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM mistakes WHERE id = ?;`, id)
	return err
}

// HELPER FUNCTIONS
func mistakeFromForm(r *http.Request) (Mistake, error) {
	topic := strings.TrimSpace(r.FormValue("topic"))
	date := strings.TrimSpace(r.FormValue("date"))
	problem := strings.TrimSpace(r.FormValue("problem_statement"))
	missed := strings.TrimSpace(r.FormValue("what_i_missed"))
	fix := strings.TrimSpace(r.FormValue("fix_rule"))
	pattern := strings.TrimSpace(r.FormValue("pattern_to_remember"))

	if topic == "" || date == "" || problem == "" || missed == "" || fix == "" || pattern == "" {
		return Mistake{}, errText("All fields are required.")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return Mistake{}, errText("Invalid date format. Use YYYY-MM-DD.")
	}

	return Mistake{
		Topic:           topic,
		Date:            date,
		Problem:         problem,
		Missed:          missed,
		FixRule:         fix,
		PatternRemember: pattern,
	}, nil
}

type errText string
func (e errText) Error() string { return string(e) }

func parseIDParam(r *http.Request, key string) (int64, bool) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		raw = r.FormValue(key)
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func redirectBack(w http.ResponseWriter, r *http.Request) {
	back := r.Referer()
	if back == "" {
		back = "/"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

func withBasicSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
