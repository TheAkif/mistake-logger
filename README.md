# Mistake Log (Go + SQLite)

A tiny local web app to log coding/interview mistakes and the rule/pattern to avoid repeating them.  
Data is stored locally in a SQLite file (`mistakes.db`) and served at `http://127.0.0.1:8080`.

## Features

- Add mistake logs with:
  - topic, date
  - problem statement
  - what I missed
  - fix rule
  - pattern to remember
- List view (latest first)
- Search / filter (free-text, topic, date range)
- Edit / delete entries
- Export to CSV: `/export.csv` (respects current filters)
- Rule Cards view: `/rules` (fix rule + pattern for revision)

## Tech Stack

- Go (`net/http`, `html/template`)
- SQLite (file-based)
- `modernc.org/sqlite` (pure Go driver, no CGO)

## Project Structure

```
mistake-log/
  main.go
  mistakes.db            # auto-created on first run
  templates/
    index.html
    edit.html
    rules.html
  static/
    styles.css
```

## Setup & Run

```bash
go mod init mistake-log
go get modernc.org/sqlite
go run .
```

Open: `http://127.0.0.1:8080`

## Routes

- `GET /` — list + add form + filters
- `POST /add` — add a new mistake
- `GET /edit?id=123` — edit page
- `POST /edit?id=123` — save edits
- `POST /delete?id=123` — delete entry
- `GET /rules` — rule cards view (supports same filters)
- `GET /export.csv` — CSV export (supports same filters)

## Reset Data

Stop the server and delete `mistakes.db` (it will be recreated next run).
