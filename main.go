package main

import (
	"flag"
	"math/rand"
	"net/http"
	"net/url"
	"time"
	"unsafe"

	"github.com/dgraph-io/badger/v3"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"

	"github.com/pkg/errors"
)

// usage: curl -F "url=domain.com" -F "secret=75d89a1775806a456eba2452e3ff3695" http://example.com

const letterBytes = "23456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ"

var src = rand.NewSource(time.Now().UnixNano())

var baseURL *url.URL

var addr = flag.String("addr", ":5050", "the address to listen on")
var base = flag.String("base", "http://127.0.0.1:5050", "")
var secret = flag.String("secret", "75d89a1775806a456eba2452e3ff3695", "")

const (
	// letter len = 57 = 111001 -> 6 bits
	letterIdxBits = 6
	letterIdxMask = 1<<letterIdxBits - 1
	letterIdxMax  = 63 / letterIdxBits
)

func randString(n int) string {
	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}
	return *(*string)(unsafe.Pointer(&b))
}

func linksCheck(links string) error {
	u, err := url.Parse(links)
	if err != nil {
		return err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("url.Scheme must be http or https")
	}
	return nil
}

func main() {
	flag.Parse()

	var err error
	if baseURL, err = url.Parse(*base); err != nil {
		panic(err)
	}

	db, err := badger.Open(badger.DefaultOptions("./data"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	e := echo.New()
	e.Use(middleware.Logger())
	e.Logger.SetLevel(log.INFO)

	e.GET("/", func(c echo.Context) error {
		return c.HTML(http.StatusOK, "")
	})

	e.POST("/", func(c echo.Context) error {
		s := c.FormValue("secret")
		if s != *secret {
			return c.String(http.StatusForbidden, "403 Forbidden\n")
		}

		links := c.FormValue("url")
		err = linksCheck(links)
		if err != nil {
			e.Logger.Info(err)
			return c.String(http.StatusBadRequest, "400 Bad Request\n")
		}

		var key string
		_ = db.View(func(txn *badger.Txn) error {
			for {
				key = randString(6)
				_, err = txn.Get([]byte(key))
				if errors.Is(err, badger.ErrKeyNotFound) {
					break
				} else {
					c.Logger().Info("random string collision")
				}
			}
			return nil
		})

		err = db.Update(func(txn *badger.Txn) error {
			err = txn.Set([]byte(key), []byte(links))
			return err
		})
		if err != nil {
			e.Logger.Error(err)
		}

		return c.String(http.StatusCreated, baseURL.ResolveReference(&url.URL{Path: "."}).String()+key+"\n")
	})

	e.GET("/:slug", func(c echo.Context) error {
		slug := c.Param("slug")

		if len(slug) != 6 {
			return c.String(http.StatusNotFound, "404 Not Found\n")
		}

		var links []byte

		err = db.View(func(txn *badger.Txn) error {
			var item *badger.Item
			item, err = txn.Get([]byte(slug))
			if err != nil {
				return err
			}

			links, err = item.ValueCopy(nil)
			if err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			e.Logger.Error(err)
			return c.String(http.StatusNotFound, "404 Not Found\n")
		}

		return c.Redirect(http.StatusMovedPermanently, string(links)+"\n")
	})
	e.Logger.Fatal(e.Start(*addr))
}
