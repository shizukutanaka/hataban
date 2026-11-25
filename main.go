// HataBan — a local-first flag-duty (旗当番) roster builder for
// elementary-school PTA district committees.
//
// One binary. It serves a small web UI on 127.0.0.1 only and keeps
// the roster and the duty table in a single JSON file in the user's
// own data folder. It has no account, no cloud and no network client
// code: the names on the roster never leave the machine.
package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"hataban/internal/plan"
)

//go:embed web/index.html
var indexHTML []byte

const (
	version = "0.1.0"
	marker  = "HATABAN-APP"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			exit(serve())
			return
		case "export":
			exit(cliExport(os.Args[2:]))
			return
		case "version", "-version", "--version":
			fmt.Println("hataban", version)
			return
		default:
			fmt.Fprintf(os.Stderr, "hataban: unknown command %q\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
	}
	exit(serve())
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: hataban [serve|export|version]")
	fmt.Fprintln(os.Stderr, "  serve             start the web UI (default when no command is given)")
	fmt.Fprintln(os.Stderr, "  export csv [file] write the duty table out as CSV")
	fmt.Fprintln(os.Stderr, "  export ics [file] write the duty table out as iCalendar")
	fmt.Fprintln(os.Stderr, "  version           print the version")
}

func exit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "hataban:", err)
		os.Exit(1)
	}
}

// ---------- server ----------

type server struct {
	store *plan.Store
	mu    sync.Mutex
}

// stateView is everything the UI renders: the plan itself, the
// school days of the period (with holiday names for reference) and
// the per-household duty counts.
type stateView struct {
	Plan   *plan.Plan   `json:"plan"`
	Days   []dayView    `json:"days"`
	Counts []plan.Count `json:"counts"`
}

type dayView struct {
	Date    string `json:"date"`
	Weekday string `json:"weekday"`
}

func buildState(p *plan.Plan) stateView {
	v := stateView{Plan: p, Counts: p.Counts()}
	if v.Counts == nil {
		v.Counts = []plan.Count{}
	}
	v.Days = []dayView{}
	if p.Settings.Start != "" && p.Settings.End != "" {
		if days, err := plan.SchoolDays(p.Settings); err == nil {
			for _, d := range days {
				v.Days = append(v.Days, dayView{
					Date:    d.Format("2006-01-02"),
					Weekday: plan.WeekdayJP(d.Weekday()),
				})
			}
		}
	}
	return v
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, err.Error())
}

func (s *server) load() (*plan.Plan, error) { return s.store.Load() }

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		p, err := s.load()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, buildState(p))
	})
	// Save the master data (roster, spots, period). Existing
	// assignments are kept as they are.
	mux.HandleFunc("POST /api/save", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var req struct {
			Households []plan.Household `json:"households"`
			Spots      []string         `json:"spots"`
			Settings   plan.Settings    `json:"settings"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			writeErr(w, 400, err)
			return
		}
		p, err := s.load()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		p.Households = req.Households
		p.Spots = req.Spots
		p.Settings = req.Settings
		p.Normalize()
		if err := p.Validate(); err != nil {
			writeErr(w, 400, err)
			return
		}
		if err := s.store.Save(p); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, buildState(p))
	})
	// Paste-import of the roster. Names already on the roster are
	// skipped and reported, unreadable lines are reported too.
	mux.HandleFunc("POST /api/import", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			writeErr(w, 400, err)
			return
		}
		p, err := s.load()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		parsed, errs := plan.ParseImport(req.Text)
		existing := map[string]bool{}
		for _, h := range p.Households {
			existing[h.Name] = true
		}
		var skipped []string
		imported := 0
		for _, h := range parsed {
			if existing[h.Name] {
				skipped = append(skipped, h.Name)
				continue
			}
			p.Households = append(p.Households, h)
			existing[h.Name] = true
			imported++
		}
		if err := s.store.Save(p); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{
			"state":    buildState(p),
			"imported": imported,
			"skipped":  skipped,
			"errors":   errs,
		})
	})
	mux.HandleFunc("POST /api/generate", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		p, err := s.load()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		assignments, unassigned, err := plan.Generate(p)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		p.Assignments = assignments
		p.Unassigned = unassigned
		if err := s.store.Save(p); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, buildState(p))
	})
	mux.HandleFunc("POST /api/reassign", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var req struct {
			Date      string `json:"date"`
			Spot      string `json:"spot"`
			Household string `json:"household"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			writeErr(w, 400, err)
			return
		}
		p, err := s.load()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		if err := p.Reassign(req.Date, req.Spot, req.Household); err != nil {
			writeErr(w, 400, err)
			return
		}
		if err := s.store.Save(p); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, buildState(p))
	})
	mux.HandleFunc("GET /api/export.csv", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		p, err := s.load()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"hataban.csv\"")
		_, _ = w.Write(plan.ExportCSV(p))
	})
	mux.HandleFunc("GET /api/export.ics", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		p, err := s.load()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"hataban.ics\"")
		_, _ = w.Write(plan.ExportICS(p, time.Now()))
	})
	return localhostGuard(mux)
}

// localhostGuard restricts the server to genuine localhost traffic.
// Binding 127.0.0.1 alone is not enough: any web page open in the
// user's browser can POST to 127.0.0.1 endpoints, and DNS rebinding
// can read responses. The guard rejects requests whose Host is not a
// loopback hostname, and for state-changing methods requires that a
// present Origin header matches the request Host exactly.
func localhostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(r.Host); err == nil {
			host = h
		}
		switch strings.ToLower(host) {
		case "127.0.0.1", "::1", "localhost":
		default:
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			origins, present := r.Header["Origin"]
			if present {
				if len(origins) != 1 {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				u, err := url.Parse(origins[0])
				if err != nil || u.Host != r.Host {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isOurServer(port int) bool {
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return strings.Contains(string(body), marker)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func serve() error {
	store, err := plan.OpenStore()
	if err != nil {
		return err
	}
	s := &server{store: store}
	for port := 8791; port <= 8800; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			if isOurServer(port) {
				url := fmt.Sprintf("http://127.0.0.1:%d", port)
				fmt.Println("HataBan is already running at", url)
				openBrowser(url)
				return nil
			}
			continue // some other app owns this port; try the next one
		}
		url := fmt.Sprintf("http://127.0.0.1:%d", port)
		fmt.Println("HataBan", version, "—", url)
		fmt.Println("Data file:", store.Path)
		fmt.Println("Press Ctrl+C to stop.")
		openBrowser(url)
		return http.Serve(ln, s.routes())
	}
	return errors.New("no free port in 8791-8800")
}

// ---------- CLI ----------

func cliExport(args []string) error {
	if len(args) < 1 || (args[0] != "csv" && args[0] != "ics") {
		return errors.New("usage: hataban export csv|ics [file]")
	}
	st, err := plan.OpenStore()
	if err != nil {
		return err
	}
	p, err := st.Load()
	if err != nil {
		return err
	}
	var raw []byte
	if args[0] == "csv" {
		raw = plan.ExportCSV(p)
	} else {
		raw = plan.ExportICS(p, time.Now())
	}
	if len(args) > 1 {
		return os.WriteFile(args[1], raw, 0o644)
	}
	_, err = os.Stdout.Write(raw)
	return err
}
