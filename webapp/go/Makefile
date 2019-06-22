all: merged

deps:
	go get github.com/go-sql-driver/mysql
	go get github.com/gorilla/mux
	go get github.com/gorilla/sessions
	go get github.com/Songmu/strrand
	go get github.com/unrolled/render

merged: deps
	go build -o isuda isuda.go star.go type.go util.go

isuda: deps
	go build -o isuda isuda.go type.go util.go

isutar: deps
	go build -o isutar isutar.go type.go util.go

.PHONY: all deps
