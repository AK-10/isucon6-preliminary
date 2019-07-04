package main

import (
	"database/sql"
	"net/http"

	_ "github.com/go-sql-driver/mysql"
)

// var (
// 	baseUrl *url.URL
// 	db      *sql.DB
// 	re      *render.Render
// )

var (
	starCache []Star
)

func initializeStar() error {
	_, err := db.Exec("TRUNCATE star")
	return err
}

func loadStarsFromCache(keyword string) []*Star {
	var stars []*Star
	for _, s := range starCache {
		if s.Keyword == keyword {
			stars = append(stars, &s)
		}
	}
	return stars
}

func loadStars(keyword string) []*Star {
	// v := url.Values{}
	// v.Set("keyword", keyword)

	rows, err := db.Query(`SELECT * FROM star WHERE keyword = ?`, keyword)
	if err != nil && err != sql.ErrNoRows {
		panicIf(err)
		return nil
	}

	stars := make([]*Star, 0, 10)
	for rows.Next() {
		s := Star{}
		err := rows.Scan(&s.ID, &s.Keyword, &s.UserName, &s.CreatedAt)
		panicIf(err)
		stars = append(stars, &s)
	}
	rows.Close()
	return stars
}

// func starsHandler(w http.ResponseWriter, r *http.Request) {
// 	keyword := r.FormValue("keyword")
// 	rows, err := db.Query(`SELECT * FROM star WHERE keyword = ?`, keyword)
// 	if err != nil && err != sql.ErrNoRows {
// 		panicIf(err)
// 		return
// 	}

// 	stars := make([]Star, 0, 10)
// 	for rows.Next() {
// 		s := Star{}
// 		err := rows.Scan(&s.ID, &s.Keyword, &s.UserName, &s.CreatedAt)
// 		panicIf(err)
// 		stars = append(stars, s)
// 	}
// 	rows.Close()

// 	re.JSON(w, http.StatusOK, map[string][]Star{
// 		"result": stars,
// 	})
// }

func starsPostHandler(w http.ResponseWriter, r *http.Request) {
	keyword := r.FormValue("keyword")

	_, err := getEntryByKeyword(keyword)
	if err == sql.ErrNoRows {
		notFound(w)
		return
	}

	user := r.FormValue("user")

	// _, err = db.Exec(`INSERT INTO star (keyword, user_name, created_at) VALUES (?, ?, NOW())`, keyword, user)
	// panicIf(err)
	starCache = append(starCache, Star{Keyword: keyword, UserName: user})

	re.JSON(w, http.StatusOK, map[string]string{"result": "ok"})
}
