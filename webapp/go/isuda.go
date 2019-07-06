package main

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"html/template"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Songmu/strrand"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/unrolled/render"

	"github.com/gomodule/redigo/redis"
)

const (
	sessionName   = "isuda_session"
	sessionSecret = "tonymoris"
)

var (
	isutarEndpoint string
	isupamEndpoint string

	baseUrl *url.URL
	db      *sql.DB
	re      *render.Render
	store   *sessions.CookieStore

	redisPool = &redis.Pool{
		MaxIdle:     3,
		MaxActive:   0,
		IdleTimeout: 5 * time.Minute,
		Dial:        func() (redis.Conn, error) { return redis.Dial("tcp", "127.0.0.1:6379") },
	}

	errInvalidUser = errors.New("Invalid User")

	replaceWordPairs = make([]string, 0, 20000)
	kw2sha           = make(map[string]string)

	keywordCache = make([]string, 0, 10000)
)

func setName(w http.ResponseWriter, r *http.Request) error {
	session := getSession(w, r)
	userID, ok := session.Values["user_id"]
	if !ok {
		return nil
	}
	setContext(r, "user_id", userID)
	row := db.QueryRow(`SELECT name FROM user WHERE id = ?`, userID)
	user := User{}
	err := row.Scan(&user.Name)
	if err != nil {
		if err == sql.ErrNoRows {
			return errInvalidUser
		}
		panicIf(err)
	}
	setContext(r, "user_name", user.Name)
	return nil
}

func authenticate(w http.ResponseWriter, r *http.Request) error {
	if u := getContext(r, "user_id"); u != nil {
		return nil
	}
	return errInvalidUser
}

func initializeHandler(w http.ResponseWriter, r *http.Request) {
	_, err := db.Exec(`DELETE FROM entry WHERE id > 7101`)
	panicIf(err)
	err = setEntryNumToRedis(7101)
	// panicIf(err)
	err = initializeStar()
	// panicIf(err)
	err = initEntries()
	// panicIf(err)
	initReplaceWordPairs()
	initKeywordCache()
	println("Pairs num:", len(replaceWordPairs))
	re.JSON(w, http.StatusOK, map[string]string{"result": "ok"})
}

func getKeywordsByDesc() []string {
	rows, err := db.Query(`
		SELECT keyword FROM entry ORDER BY keyword_length DESC
	`)
	panicIf(err)

	var kws []string
	for rows.Next() {
		var kw string
		err := rows.Scan(&kw)
		panicIf(err)
		kws = append(kws, kw)
	}
	defer rows.Close()
	return kws
}

func topHandler(w http.ResponseWriter, r *http.Request) {
	if err := setName(w, r); err != nil {
		forbidden(w)
		return
	}

	// keywords := getKeywordsByDesc()

	perPage := 10
	p := r.URL.Query().Get("page")
	if p == "" {
		p = "1"
	}
	page, _ := strconv.Atoi(p)

	rows, err := db.Query(
		"SELECT * FROM entry ORDER BY updated_at DESC LIMIT ? OFFSET ?",
		perPage, perPage*(page-1),
	)

	if err != nil && err != sql.ErrNoRows {
		panicIf(err)
	}
	entries := make([]*Entry, 0, 10)
	for rows.Next() {
		e := Entry{}
		err := rows.Scan(&e.ID, &e.AuthorID, &e.Keyword, &e.Description, &e.UpdatedAt, &e.CreatedAt, &e.KeywordLength)
		panicIf(err)
		html := htmlify(w, r, e.Description, keywordCache)
		// html, err := getHTMLOfEntryfromRedis(e.Keyword)
		// if err == redis.ErrNil {
		// 	html = htmlify(w, r, e.Description, keywords)
		// 	err = setHTMLOfEntryToRedis(e.Keyword, html)
		// 	panicIf(err)
		// }
		panicIf(err)
		e.Html = html
		// e.Stars = loadStars(e.Keyword)
		e.Stars = loadStarsFromCache(e.Keyword)
		entries = append(entries, &e)
	}

	rows.Close()

	// var totalEntries int
	// row := db.QueryRow(`SELECT COUNT(*) FROM entry`)
	// err = row.Scan(&totalEntries)
	// if err != nil && err != sql.ErrNoRows {
	// 	panicIf(err)
	// }
	totalEntries, err := getEntryNumFromRedis()
	if err != nil {
		panicIf(err)
	}

	lastPage := int(math.Ceil(float64(totalEntries) / float64(perPage)))
	pages := make([]int, 0, 10)
	start := int(math.Max(float64(1), float64(page-5)))
	end := int(math.Min(float64(lastPage), float64(page+5)))
	for i := start; i <= end; i++ {
		pages = append(pages, i)
	}

	re.HTML(w, http.StatusOK, "index", struct {
		Context  context.Context
		Entries  []*Entry
		Page     int
		LastPage int
		Pages    []int
	}{
		r.Context(), entries, page, lastPage, pages,
	})
}

func robotsHandler(w http.ResponseWriter, r *http.Request) {
	notFound(w)
}

func keywordPostHandler(w http.ResponseWriter, r *http.Request) {
	if err := setName(w, r); err != nil {
		forbidden(w)
		return
	}
	if err := authenticate(w, r); err != nil {
		forbidden(w)
		return
	}

	keyword := r.FormValue("keyword")
	if keyword == "" {
		badRequest(w)
		return
	}
	userID := getContext(r, "user_id").(int)
	description := r.FormValue("description")

	if isSpamContents(description) || isSpamContents(keyword) {
		http.Error(w, "SPAM!", http.StatusBadRequest)
		return
	}

	_, err := db.Exec(`
		INSERT INTO entry (author_id, keyword, description, created_at, updated_at)
		VALUES (?, ?, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE
		author_id = ?, keyword = ?, description = ?, updated_at = NOW()
	`, userID, keyword, description, userID, keyword, description)
	if err == nil {
		incEntryNum()
		keywordCache = append([]string{keyword}, keywordCache...)
	}
	panicIf(err)

	http.Redirect(w, r, "/", http.StatusFound)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if err := setName(w, r); err != nil {
		forbidden(w)
		return
	}

	re.HTML(w, http.StatusOK, "authenticate", struct {
		Context context.Context
		Action  string
	}{
		r.Context(), "login",
	})
}

func loginPostHandler(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	row := db.QueryRow(`SELECT * FROM user WHERE name = ?`, name)
	user := User{}
	err := row.Scan(&user.ID, &user.Name, &user.Salt, &user.Password, &user.CreatedAt)
	if err == sql.ErrNoRows || user.Password != fmt.Sprintf("%x", sha1.Sum([]byte(user.Salt+r.FormValue("password")))) {
		forbidden(w)
		return
	}
	panicIf(err)
	session := getSession(w, r)
	session.Values["user_id"] = user.ID
	session.Save(r, w)
	http.Redirect(w, r, "/", http.StatusFound)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	session := getSession(w, r)
	session.Options = &sessions.Options{MaxAge: -1}
	session.Save(r, w)
	http.Redirect(w, r, "/", http.StatusFound)
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	if err := setName(w, r); err != nil {
		forbidden(w)
		return
	}

	re.HTML(w, http.StatusOK, "authenticate", struct {
		Context context.Context
		Action  string
	}{
		r.Context(), "register",
	})
}

func registerPostHandler(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	pw := r.FormValue("password")
	if name == "" || pw == "" {
		badRequest(w)
		return
	}
	userID := register(name, pw)
	session := getSession(w, r)
	session.Values["user_id"] = userID
	session.Save(r, w)
	http.Redirect(w, r, "/", http.StatusFound)
}

func register(user string, pass string) int64 {
	salt, err := strrand.RandomString(`....................`)
	panicIf(err)
	res, err := db.Exec(`INSERT INTO user (name, salt, password, created_at) VALUES (?, ?, ?, NOW())`,
		user, salt, fmt.Sprintf("%x", sha1.Sum([]byte(salt+pass))))
	panicIf(err)
	lastInsertID, _ := res.LastInsertId()
	return lastInsertID
}

func initEntries() error {
	_, err := db.Exec(`UPDATE entry SET keyword_length = CHAR_LENGTH(keyword)`)
	return err
}

func initKeywordCache() {
	keywordCache = getKeywordsByDesc()
}

func initReplaceWordPairs() {
	keywords := getKeywordsByDesc()
	for _, kw := range keywords {
		kw = regexp.QuoteMeta(kw)
		kw2sha[kw] = "isuda_" + fmt.Sprintf("%x", sha1.Sum([]byte(kw)))
		replaceWordPairs = append(replaceWordPairs, kw)
		replaceWordPairs = append(replaceWordPairs, kw2sha[kw])
	}
}

func getEntryByKeyword(kw string) (Entry, error) {
	row := db.QueryRow(`SELECT * FROM entry WHERE keyword = ?`, kw)
	e := Entry{}
	err := row.Scan(&e.ID, &e.AuthorID, &e.Keyword, &e.Description, &e.UpdatedAt, &e.CreatedAt, &e.KeywordLength)
	return e, err
}

func keywordByKeywordHandler(w http.ResponseWriter, r *http.Request) {
	if err := setName(w, r); err != nil {
		forbidden(w)
		return
	}

	keyword, _ := url.QueryUnescape(mux.Vars(r)["keyword"])

	e, err := getEntryByKeyword(keyword)
	if err == sql.ErrNoRows {
		notFound(w)
		return
	}

	// keywords := getKeywordsByDesc()
	keywords := keywordCache
	html := htmlify(w, r, e.Description, keywords)
	// html, err := getHTMLOfEntryfromRedis(e.Keyword)
	// if err == redis.ErrNil {
	// 	html = htmlify(w, r, e.Description, keywords)
	// 	err = setHTMLOfEntryToRedis(e.Keyword, html)
	// 	panicIf(err)
	// }
	panicIf(err)
	e.Html = html
	e.Stars = loadStarsFromCache(e.Keyword)
	// e.Stars = loadStars(e.Keyword)

	re.HTML(w, http.StatusOK, "keyword", struct {
		Context context.Context
		Entry   Entry
	}{
		r.Context(), e,
	})
}

func keywordByKeywordDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if err := setName(w, r); err != nil {
		forbidden(w)
		return
	}
	if err := authenticate(w, r); err != nil {
		forbidden(w)
		return
	}

	keyword := mux.Vars(r)["keyword"]
	if keyword == "" {
		badRequest(w)
		return
	}
	if r.FormValue("delete") == "" {
		badRequest(w)
		return
	}
	row := db.QueryRow(`SELECT * FROM entry WHERE keyword = ?`, keyword)
	e := Entry{}
	err := row.Scan(&e.ID, &e.AuthorID, &e.Keyword, &e.Description, &e.UpdatedAt, &e.CreatedAt, &e.KeywordLength)
	if err == sql.ErrNoRows {
		notFound(w)
		return
	}
	panicIf(err)

	_, err = db.Exec(`DELETE FROM entry WHERE keyword = ?`, keyword)
	if err == nil {
		decEntryNum()
		deleteKeywordFromCache(keyword)
		deleteWordPairFromCache(keyword)
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func deleteKeywordFromCache(keyword string) {
	for i, k := range keywordCache {
		if k == keyword {
			keywordCache = append(keywordCache[:i], keywordCache[i+1:]...)
		}
		break
	}
}

func deleteWordPairFromCache(keyword string) {
	for i, k := range keywordCache {
		if k == keyword {
			replaceWordPairs = append(replaceWordPairs[:i], replaceWordPairs[i+2:]...)
			delete(kw2sha, keyword)
		}
		break
	}
}

func htmlify(w http.ResponseWriter, r *http.Request, content string, keywords []string) string {
	if content == "" {
		return ""
	}

	rs := strings.NewReplacer(replaceWordPairs...)
	content = rs.Replace(content)
	content = html.EscapeString(content)
	for kw, hash := range kw2sha {
		u, err := r.URL.Parse(baseUrl.String() + "/keyword/" + pathURIEscape(kw))
		panicIf(err)
		link := fmt.Sprintf("<a href=\"%s\">%s</a>", u, html.EscapeString(kw))
		content = strings.Replace(content, hash, link, -1)
	}

	return strings.Replace(content, "\n", "<br />\n", -1)
}

func isSpamContents(content string) bool {
	v := url.Values{}
	v.Set("content", content)
	resp, err := http.PostForm(isupamEndpoint, v)
	panicIf(err)
	defer resp.Body.Close()

	var data struct {
		Valid bool `json:valid`
	}
	err = json.NewDecoder(resp.Body).Decode(&data)
	panicIf(err)
	return !data.Valid
}

func getContext(r *http.Request, key interface{}) interface{} {
	return r.Context().Value(key)
}

func setContext(r *http.Request, key, val interface{}) {
	if val == nil {
		return
	}

	r2 := r.WithContext(context.WithValue(r.Context(), key, val))
	*r = *r2
}

func getSession(w http.ResponseWriter, r *http.Request) *sessions.Session {
	session, _ := store.Get(r, sessionName)
	return session
}

func main() {
	host := os.Getenv("ISUDA_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	portstr := os.Getenv("ISUDA_DB_PORT")
	if portstr == "" {
		portstr = "3306"
	}
	port, err := strconv.Atoi(portstr)
	if err != nil {
		log.Fatalf("Failed to read DB port number from an environment variable ISUDA_DB_PORT.\nError: %s", err.Error())
	}
	user := os.Getenv("ISUDA_DB_USER")
	if user == "" {
		user = "root"
	}
	password := os.Getenv("ISUDA_DB_PASSWORD")
	dbname := os.Getenv("ISUDA_DB_NAME")
	if dbname == "" {
		dbname = "isuda"
	}

	db, err = sql.Open("mysql", fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?loc=Local&parseTime=true",
		user, password, host, port, dbname,
	))
	if err != nil {
		log.Fatalf("Failed to connect to DB: %s.", err.Error())
	}
	db.Exec("SET SESSION sql_mode='TRADITIONAL,NO_AUTO_VALUE_ON_ZERO,ONLY_FULL_GROUP_BY'")
	db.Exec("SET NAMES utf8mb4")

	isutarEndpoint = os.Getenv("ISUTAR_ORIGIN")
	if isutarEndpoint == "" {
		isutarEndpoint = "http://localhost:5001"
	}
	isupamEndpoint = os.Getenv("ISUPAM_ORIGIN")
	if isupamEndpoint == "" {
		isupamEndpoint = "http://localhost:5050"
	}

	store = sessions.NewCookieStore([]byte(sessionSecret))

	re = render.New(render.Options{
		Directory: "views",
		Funcs: []template.FuncMap{
			{
				"url_for": func(path string) string {
					return baseUrl.String() + path
				},
				"title": func(s string) string {
					return strings.Title(s)
				},
				"raw": func(text string) template.HTML {
					return template.HTML(text)
				},
				"add": func(a, b int) int { return a + b },
				"sub": func(a, b int) int { return a - b },
				"entry_with_ctx": func(entry Entry, ctx context.Context) *EntryWithCtx {
					return &EntryWithCtx{Context: ctx, Entry: entry}
				},
			},
		},
	})

	r := mux.NewRouter()
	r.UseEncodedPath()
	r.HandleFunc("/", myHandler(topHandler))
	r.HandleFunc("/initialize", myHandler(initializeHandler)).Methods("GET")
	r.HandleFunc("/robots.txt", myHandler(robotsHandler))
	r.HandleFunc("/keyword", myHandler(keywordPostHandler)).Methods("POST")

	l := r.PathPrefix("/login").Subrouter()
	l.Methods("GET").HandlerFunc(myHandler(loginHandler))
	l.Methods("POST").HandlerFunc(myHandler(loginPostHandler))
	r.HandleFunc("/logout", myHandler(logoutHandler))

	g := r.PathPrefix("/register").Subrouter()
	g.Methods("GET").HandlerFunc(myHandler(registerHandler))
	g.Methods("POST").HandlerFunc(myHandler(registerPostHandler))

	k := r.PathPrefix("/keyword/{keyword}").Subrouter()
	k.Methods("GET").HandlerFunc(myHandler(keywordByKeywordHandler))
	k.Methods("POST").HandlerFunc(myHandler(keywordByKeywordDeleteHandler))

	s := r.PathPrefix("/stars").Subrouter()
	// s.Methods("GET").HandlerFunc(myHandler(starsHandler))
	s.Methods("POST").HandlerFunc(myHandler(starsPostHandler))

	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./public/")))
	log.Fatal(http.ListenAndServe(":5000", r))
}
