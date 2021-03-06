// Copyright © 2014-2015 Thomas Rabaix <thomas.rabaix@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"bufio"
	"container/list"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/schema"
	"github.com/gorilla/websocket"
	"github.com/lib/pq"
	"github.com/rande/goapp"
	"github.com/rande/gonode/core"
	"github.com/rande/gonode/core/config"
	"github.com/rande/gonode/helper"
	"github.com/rande/gonode/plugins/search"
	"github.com/rande/gonode/plugins/user"
	"github.com/zenazn/goji/graceful"
	"github.com/zenazn/goji/web"
	"golang.org/x/crypto/bcrypt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func readLoop(c *websocket.Conn) {
	for {
		if _, _, err := c.NextReader(); err != nil {
			return
		}
	}
}

func ConfigureServer(l *goapp.Lifecycle, conf *config.ServerConfig) {

	l.Prepare(func(app *goapp.App) error {
		app.Set("gonode.websocket.clients", func(app *goapp.App) interface{} {
			return list.New()
		})

		sub := app.Get("gonode.postgres.subscriber").(*core.Subscriber)
		sub.ListenMessage(conf.Databases["master"].Prefix+"_manager_action", func(notification *pq.Notification) (int, error) {
			logger := app.Get("logger").(*log.Logger)
			logger.Printf("WebSocket: Sending message \n")
			webSocketList := app.Get("gonode.websocket.clients").(*list.List)

			for e := webSocketList.Front(); e != nil; e = e.Next() {
				if err := e.Value.(*websocket.Conn).WriteMessage(websocket.TextMessage, []byte(notification.Extra)); err != nil {
					logger.Printf("Error writing to websocket")
				}
			}

			logger.Printf("WebSocket: End Sending message \n")

			return core.PubSubListenContinue, nil
		})

		graceful.PreHook(func() {
			logger := app.Get("logger").(*log.Logger)
			webSocketList := app.Get("gonode.websocket.clients").(*list.List)

			logger.Printf("Closing websocket connections \n")
			for e := webSocketList.Front(); e != nil; e = e.Next() {
				e.Value.(*websocket.Conn).Close()
			}
		})

		return nil
	})

	l.Run(func(app *goapp.App, state *goapp.GoroutineState) error {
		logger := app.Get("logger").(*log.Logger)
		logger.Printf("Starting PostgreSQL subcriber \n")
		app.Get("gonode.postgres.subscriber").(*core.Subscriber).Register()

		return nil
	})

	l.Exit(func(app *goapp.App) error {
		logger := app.Get("logger").(*log.Logger)
		logger.Printf("Closing PostgreSQL subcriber \n")
		app.Get("gonode.postgres.subscriber").(*core.Subscriber).Stop()
		logger.Printf("End closing PostgreSQL subcriber \n")

		return nil
	})

	l.Prepare(func(app *goapp.App) error {
		mux := app.Get("goji.mux").(*web.Mux)
		manager := app.Get("gonode.manager").(*core.PgNodeManager)
		apiHandler := app.Get("gonode.api").(*Api)
		handler_collection := app.Get("gonode.handler_collection").(core.Handlers)
		searchBuilder := app.Get("gonode.search.pgsql").(*search.SearchPGSQL)
		searchParser := app.Get("gonode.search.parser.http").(*search.HttpSearchParser)
		prefix := ""

		mux.Get(prefix+"/hello", func(c web.C, res http.ResponseWriter, req *http.Request) {
			res.Write([]byte("Hello!"))
		})

		mux.Post(prefix+"/login", func(c web.C, res http.ResponseWriter, req *http.Request) {
			res.Header().Set("Content-Type", "application/json")

			req.ParseForm()

			loginForm := &struct {
				Username string `schema:"username"`
				Password string `schema:"password"`
			}{}

			decoder := schema.NewDecoder()
			err := decoder.Decode(loginForm, req.Form)

			core.PanicOnError(err)

			query := manager.SelectBuilder(core.NewSelectOptions()).Where("type = 'core.user' AND data->>'username' = ?", loginForm.Username)

			node := manager.FindOneBy(query)

			password := []byte("$2a$10$KDobsZdRDVnuMqvimYH82.Tnu3suk5xP7QzhQjlCo7Wy7d67xtYay")

			if node != nil {
				data := node.Data.(*user.User)
				password = []byte(data.Password)
			}

			fmt.Printf("%s => %s", password, loginForm.Password)

			if err := bcrypt.CompareHashAndPassword([]byte(password), []byte(loginForm.Password)); err == nil { // equal
				token := jwt.New(jwt.SigningMethodHS256)
				token.Header["kid"] = "the sha1"

				// Set some claims
				token.Claims["exp"] = time.Now().Add(time.Hour * 72).Unix()
				// Sign and get the complete encoded token as a string
				tokenString, err := token.SignedString([]byte(conf.Guard.Key))

				if err != nil {
					helper.SendWithHttpCode(res, http.StatusInternalServerError, "Unable to sign the token")
					return
				}

				core.PanicOnError(err)
				res.Write([]byte(tokenString))
			} else {
				helper.SendWithHttpCode(res, http.StatusForbidden, "Unable to authenticate request: "+err.Error())
			}
		})

		mux.Get(prefix+"/nodes/stream", func(res http.ResponseWriter, req *http.Request) {
			webSocketList := app.Get("gonode.websocket.clients").(*list.List)

			upgrader.CheckOrigin = func(r *http.Request) bool {
				return true
			}

			ws, err := upgrader.Upgrade(res, req, nil)

			core.PanicOnError(err)

			element := webSocketList.PushBack(ws)

			var closeDefer = func() {
				ws.Close()
				webSocketList.Remove(element)
			}

			defer closeDefer()

			go readLoop(ws)

			// ping remote client, avoid keeping open connection
			for {
				time.Sleep(2 * time.Second)
				if err := ws.WriteMessage(websocket.TextMessage, []byte("PING")); err != nil {
					return
				}
			}
		})

		mux.Get(prefix+"/nodes/:uuid", func(c web.C, res http.ResponseWriter, req *http.Request) {
			values := req.URL.Query()

			if _, raw := values["raw"]; raw { // ask for binary content
				reference, err := core.GetReferenceFromString(c.URLParams["uuid"])

				if err != nil {
					helper.SendWithHttpCode(res, http.StatusInternalServerError, "Unable to parse the reference")

					return
				}

				node := manager.Find(reference)

				if node == nil {
					helper.SendWithHttpCode(res, http.StatusNotFound, "Element not found")

					return
				}

				data := handler_collection.Get(node).GetDownloadData(node)

				res.Header().Set("Content-Type", data.ContentType)

				//			if download {
				//				res.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", data.Filename));
				//			}

				data.Stream(node, res)
			} else {
				// send the json value
				res.Header().Set("Content-Type", "application/json")
				err := apiHandler.FindOne(c.URLParams["uuid"], res)

				if err == core.NotFoundError {
					helper.SendWithHttpCode(res, http.StatusNotFound, err.Error())
				}

				if err != nil {
					helper.SendWithHttpCode(res, http.StatusInternalServerError, err.Error())
				}
			}
		})

		mux.Get(prefix+"/nodes/:uuid/revisions", func(c web.C, res http.ResponseWriter, req *http.Request) {
			res.Header().Set("Content-Type", "application/json")

			searchForm := searchParser.HandleSearch(res, req)

			options := core.NewSelectOptions()
			options.TableSuffix = "nodes_audit"

			query := apiHandler.SelectBuilder(options).
				Where("uuid = ?", c.URLParams["uuid"])

			apiHandler.Find(res, searchBuilder.BuildQuery(searchForm, query), searchForm.Page, searchForm.PerPage)
		})

		mux.Get(prefix+"/nodes/:uuid/revisions/:rev", func(c web.C, res http.ResponseWriter, req *http.Request) {
			res.Header().Set("Content-Type", "application/json")

			options := core.NewSelectOptions()
			options.TableSuffix = "nodes_audit"

			query := apiHandler.SelectBuilder(options).
				Where("uuid = ?", c.URLParams["uuid"]).
				Where("revision = ?", c.URLParams["rev"])

			err := apiHandler.FindOneBy(query, res)

			if err == core.NotFoundError {
				helper.SendWithHttpCode(res, http.StatusNotFound, err.Error())
			}

			if err != nil {
				helper.SendWithHttpCode(res, http.StatusInternalServerError, err.Error())
			}
		})

		mux.Post(prefix+"/nodes", func(res http.ResponseWriter, req *http.Request) {
			res.Header().Set("Content-Type", "application/json")

			w := bufio.NewWriter(res)

			err := apiHandler.Save(req.Body, w)

			if err == core.RevisionError {
				res.WriteHeader(http.StatusConflict)
			}

			if err == core.ValidationError {
				res.WriteHeader(http.StatusPreconditionFailed)
			}

			res.WriteHeader(http.StatusCreated)

			w.Flush()
		})

		mux.Put(prefix+"/nodes/:uuid", func(c web.C, res http.ResponseWriter, req *http.Request) {
			res.Header().Set("Content-Type", "application/json")

			values := req.URL.Query()

			if _, raw := values["raw"]; raw { // send binary data
				reference, err := core.GetReferenceFromString(c.URLParams["uuid"])

				if err != nil {
					helper.SendWithHttpCode(res, http.StatusInternalServerError, "Unable to parse the reference")

					return
				}

				node := manager.Find(reference)

				if node == nil {
					helper.SendWithHttpCode(res, http.StatusNotFound, "Element not found")
					return
				}

				_, err = handler_collection.Get(node).StoreStream(node, req.Body)

				if err != nil {
					helper.SendWithHttpCode(res, http.StatusInternalServerError, err.Error())
				} else {
					manager.Save(node, false)

					helper.SendWithHttpCode(res, http.StatusOK, "binary stored")
				}

			} else {
				w := bufio.NewWriter(res)

				err := apiHandler.Save(req.Body, w)

				if err == core.RevisionError {
					res.WriteHeader(http.StatusConflict)
				}

				if err == core.ValidationError {
					res.WriteHeader(http.StatusPreconditionFailed)
				}

				w.Flush()
			}
		})

		mux.Put(prefix+"/nodes/move/:uuid/:parentUuid", func(c web.C, res http.ResponseWriter, req *http.Request) {
			res.Header().Set("Content-Type", "application/json")

			err := apiHandler.Move(c.URLParams["uuid"], c.URLParams["parentUuid"], res)

			if err != nil {
				helper.SendWithHttpCode(res, http.StatusInternalServerError, err.Error())
			}
		})

		mux.Delete(prefix+"/nodes/:uuid", func(c web.C, res http.ResponseWriter, req *http.Request) {
			err := apiHandler.RemoveOne(c.URLParams["uuid"], res)

			if err == core.NotFoundError {
				helper.SendWithHttpCode(res, http.StatusNotFound, err.Error())
				return
			}

			if err == core.AlreadyDeletedError {
				helper.SendWithHttpCode(res, http.StatusGone, err.Error())
				return
			}

			if err != nil {
				helper.SendWithHttpCode(res, http.StatusInternalServerError, err.Error())
			}
		})

		mux.Put(prefix+"/notify/:name", func(c web.C, res http.ResponseWriter, req *http.Request) {
			body, _ := ioutil.ReadAll(req.Body)

			manager.Notify(c.URLParams["name"], string(body[:]))
		})

		mux.Get(prefix+"/nodes", func(c web.C, res http.ResponseWriter, req *http.Request) {
			res.Header().Set("Content-Type", "application/json")

			searchForm := searchParser.HandleSearch(res, req)

			if searchForm == nil {
				return
			}

			query := searchBuilder.BuildQuery(searchForm, manager.SelectBuilder(core.NewSelectOptions()))

			apiHandler.Find(res, query, searchForm.Page, searchForm.PerPage)
		})

		return nil
	})
}
