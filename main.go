package main

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type Mistake struct {
	ID              int64
	Topic           string
	Date            string // YYYY-MM-DD
	Problem         string
	Missed          string
	FixRule         string
	PatternRemember string
	CreatedAt       string
}

type PageData struct {
	Today    string
	Mistakes []Mistake
}

var (
	db   *sql.DB
	tmpl *template.Template
)

func main() {
	var err error

	// Open or create SQLite database file in the current directory
	db, err = sql.Open("sqlite", "mistakes.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := initSchema(db); err != nil {
		log.Fatal(err)
	}

	// Parse templates
	tmpl, err = template.ParseFiles(filepath.Join("templates", "index.html"))
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/add", handleAdd)
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
`
	_, err := db.Exec(schema)
	return err
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mistakes, err := listMistakes(db)
	if err != nil {
		http.Error(w, "Failed to load mistakes", http.StatusInternalServerError)
		log.Println("listMistakes:", err)
		return
	}

	data := PageData{
		Today:    time.Now().Format("2006-01-02"),
		Mistakes: mistakes,
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Println("template execute:", err)
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

	// Trim + basic validation
	topic := strings.TrimSpace(r.FormValue("topic"))
	date := strings.TrimSpace(r.FormValue("date"))
	problem := strings.TrimSpace(r.FormValue("problem_statement"))
	missed := strings.TrimSpace(r.FormValue("what_i_missed"))
	fix := strings.TrimSpace(r.FormValue("fix_rule"))
	pattern := strings.TrimSpace(r.FormValue("pattern_to_remember"))

	if topic == "" || date == "" || problem == "" || missed == "" || fix == "" || pattern == "" {
		http.Error(w, "All fields are required.", http.StatusBadRequest)
		return
	}
	// Ensure date is a valid YYYY-MM-DD
	if _, err := time.Parse("2006-01-02", date); err != nil {
		http.Error(w, "Invalid date format. Use YYYY-MM-DD.", http.StatusBadRequest)
		return
	}

	if err := insertMistake(db, Mistake{
		Topic:           topic,
		Date:            date,
		Problem:         problem,
		Missed:          missed,
		FixRule:         fix,
		PatternRemember: pattern,
		CreatedAt:       time.Now().Format(time.RFC3339),
	}); err != nil {
		http.Error(w, "Failed to save mistake", http.StatusInternalServerError)
		log.Println("insertMistake:", err)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func listMistakes(db *sql.DB) ([]Mistake, error) {
	rows, err := db.Query(`
SELECT id, topic, date, problem_statement, what_i_missed, fix_rule, pattern_to_remember, created_at
FROM mistakes
ORDER BY date DESC, id DESC
LIMIT 200;
`)
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

func withBasicSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
