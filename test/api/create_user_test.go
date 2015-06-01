package api

import (
	. "github.com/rande/goapp"
	nc "github.com/rande/gonode/core"
	"github.com/rande/gonode/extra"
	nh "github.com/rande/gonode/handlers"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"os"
	"testing"
)

func Test_Create_User(t *testing.T) {
	var res *extra.Response
	var app *App

	app = extra.GetApp("../config_test.toml")

	ts := app.Get("testserver").(*httptest.Server)

	file, _ := os.Open("../fixtures/new_user.json")

	res, _ = extra.RunRequest("POST", ts.URL+"/nodes", file)

	node := nc.NewNode()
	serializer := app.Get("gonode.node.serializer").(*nc.Serializer)
	serializer.Deserialize(res.Body, node)

	assert.Equal(t, node.Type, "core.user")

	user := node.Data.(*nh.User)

	assert.Equal(t, user.FirstName, "User")
	assert.Equal(t, user.LastName, "12")
}
