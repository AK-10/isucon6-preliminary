package main

import (
	"strconv"

	"github.com/gomodule/redigo/redis"
)

const (
	entryNumKey   = "entryNum"
	htmlKeyPrefix = "HTML-OF-"
	// starPrefix = "STAR-"
)

func flushAllHTML() error {
	conn := redisPool.Get()
	defer conn.Close()
	entryNum, err := getEntryNumFromRedis()
	if err != nil {
		return err
	}
	_, err = conn.Do("FLUSHALL")
	if err != nil {
		return err
	}

	err = setEntryNumToRedis(entryNum)
	if err != nil {
		return err
	}
	return nil
}

func setStarWith

func setHTMLOfEntryToRedis(keyword string, html string) error {
	conn := redisPool.Get()
	defer conn.Close()
	key := htmlKeyPrefix + keyword
	_, err := conn.Do("SET", key, html)
	return err
}

func getHTMLOfEntryfromRedis(keyword string) (string, error) {
	conn := redisPool.Get()
	defer conn.Close()

	key := htmlKeyPrefix + keyword

	html, err := redis.String(conn.Do("GET", key))
	return html, err
}

func setEntryNumToRedis(num int64) error {
	conn := redisPool.Get()
	defer conn.Close()
	_, err := conn.Do("SET", entryNumKey, strconv.FormatInt(num, 10))
	return err
}

func getEntryNumFromRedis() (int64, error) {
	conn := redisPool.Get()
	defer conn.Close()
	num, err := redis.Int64(conn.Do("GET", entryNumKey))
	return num, err
}

func incEntryNum() {
	conn := redisPool.Get()
	defer conn.Close()
	num, err := redis.Int64(conn.Do("GET", entryNumKey))
	panicIf(err)
	_, err = conn.Do("SET", entryNumKey, strconv.FormatInt(num+1, 10))
	panicIf(err)
}

func decEntryNum() {
	conn := redisPool.Get()
	defer conn.Close()
	num, err := redis.Int64(conn.Do("GET", entryNumKey))
	panicIf(err)
	_, err = conn.Do("SET", entryNumKey, strconv.FormatInt(num-1, 10))
	panicIf(err)
}
